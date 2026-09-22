# Packaging and shipping

How framefairy becomes something a person downloads, installs and pays for.
The plan with statuses is phase 5 of [GUI-PLAN.md](GUI-PLAN.md). This page
is the reasoning behind it, so a decision does not have to be made twice.

## What is already solved

There is no JavaScript to ship. `cmd/framefairy-app/main.go` embeds the built
interface with `go:embed`, so the Svelte app is inside the binary. Nothing
needs Node at runtime, and the webview is the system's own: WebKit on macOS,
WebKitGTK on Linux, WebView2 on Windows. That is the whole reason for Wails
and it is the part that is finished.

## Decided

### 1. macOS first

It is the machine that can be tested. Windows and Linux follow once the
shape is proven, and they are a build problem rather than a design problem
by then.

macOS is also the strictest gate, so it teaches the most: a signed
Developer ID build, notarised by Apple, or Gatekeeper refuses to open it.

### 2. No language model ships

The download would be 14.4 GB and it wants about 18 GB of free memory. That
is not an installer, and it is not most machines.

So finding clips is a choice the user makes, once, and the app asks it
plainly:

- **An Anthropic API key.** Works on any machine, costs per episode, nothing
  to install. This is the path that gets somebody to their first short in
  minutes.
- **A local model.** Free per run, private, needs the machine for it. The
  app walks them through getting it: four models from three houses, each
  saying what it costs to fetch and what it costs in memory to run, with
  the one this machine should have marked as such. A download nobody agreed
  to is a download nobody wanted, so every figure is said before anything
  starts.

Speech is not part of that choice. Transcription is always local, always
Parakeet, and the speech model is 490 MB, which is a first-run download
nobody has to think about.

**This is built.** The first run is a setup screen, `frontend/src/screens/
Setup.svelte`. It fetches the speech model by itself, asks the one question,
and on the local side offers the language models with what each costs to
fetch, what it costs in memory, and what this machine can do with it. The
key goes in the macOS keychain. Both installers land under a part name and
only move a whole model into place, so a cancel leaves nothing rather than
half of something. See [APP.md](APP.md#the-first-run).

Two things about that list are worth writing down.

**A model is judged by memory, not by disk.** It runs from memory, so a
machine too small for one swaps, and a model that swaps takes minutes to
answer rather than seconds. Each entry says what it needs to run, and the
app reads what the machine has and says whether it fits, is tight, or is
too big. It never refuses: a machine's memory can be read wrong and it is
not the app's place to decide, so it says what it thinks before the
download rather than after it.

**Every model is pinned and every model comes from its own maker.** The
sizes and the checksums are read off the real files through the Hugging
Face API rather than guessed, which matters: the first one was guessed once
and was a gigabyte out. Nothing on the list is somebody else's quantisation
of somebody's model, so what is fetched is what its maker meant to release,
and its licence is between the user and that maker. On top of the checksum
the installer checks that what arrived begins with `GGUF`, because a page
saying no, saved under a model's name, is the thing that gets past
everything else.

**What is not done yet: a download does not resume.** A language model is
between five and fifteen gigabytes, and one that fails at nine tenths
starts again from nothing. The part file is already there to resume from
and the server supports it, so this is a range request and a checksum fed
the bytes that are already on disk.

**A consequence worth having.** Because no model ships, we never
redistribute one. The app fetches a model from whoever published it, the way
`scripts/models.sh` does today, so its licence is between the user and Google
rather than something we have to carry.

### 3. ffmpeg: an LGPL build, without libx264, encoding through the system

This was the open question and the answer turned out to be smaller than it
looked.

**What is actually GPL.** Not ffmpeg. ffmpeg itself is LGPL. It becomes GPL
only when it is configured with `--enable-gpl`, and that flag is there to
allow GPL-licensed external encoders. In practice, for us, that is exactly
one thing: **libx264**. Everything else the engine asks for is outside it.
libass, which burns in the captions, is ISC licensed and does not force
anything.

So the whole problem is one encoder, and the encoder is the one part every
system already has:

| System | H.264 encoder to use instead |
| --- | --- |
| macOS | `h264_videotoolbox`, Apple's, hardware |
| Windows | `h264_mf`, or NVENC and QSV where they exist |
| Linux | VA-API, or openh264 |

On an M2 Max this is also **faster** than libx264, because it is the media
engine rather than the cores.

**This is built**, by `scripts/build-ffmpeg.sh`. `make` runs it the first
time, because it takes many minutes and its answer changes only when that
script does, and every build after it copies what it left beside the
programs. From then on the development build uses the ffmpeg a customer
will use rather than whatever Homebrew has installed. `make ffmpeg` builds
it again from scratch, for when the script has changed.

Run on Linux, which proves everything but the parts that are Apple's:

| Checked | Result |
| --- | --- |
| `ffmpeg -L` | GNU **Lesser** General Public License |
| libx264 | absent, which is what makes the first line true |
| Size | 24 MB, static, one file with nothing to chase |
| Captions | a real caption file and the bundled face burned in, 41042 pixels of ink |
| H.264 on Linux | **none at all** |

That last row is the open question from above, answered as fact rather than
guess. With no libx264 and nothing detected, an LGPL ffmpeg on Linux cannot
write H.264. macOS is fine, because `--enable-videotoolbox` builds Apple's
encoder in, and that is the one that ships first. Linux needs VA-API or
openh264 added before it can ship at all, and the app says so plainly now:
Preflight refuses with the encoders it looked for.

**What this leaves to do.** Ship that ffmpeg as a plain executable inside
the app bundle. Distributing an unmodified LGPL binary asks for two things: the
licence text travels with it, and the source is available to anyone who
asks. That is a file in the repository and a tarball on a download page. No
lawyer, no ambiguity, no judgement call about where one work ends and
another begins.

**What does not change.** The engine, the command line and the filter graph
all stay as they are. ffmpeg does five separate jobs for us and only one of
them is touched:

| Job | Where |
| --- | --- |
| Render a clip: seek, crop, scale, pad, audio fades, concat, burn in captions, encode | `engine/render.go` |
| Read the episode's shape, frame rate and colour tags | `engine/ffmpeg.go` |
| Still frames for the video preview | `engine/frames.go` |
| Scene cuts and the grey stream the face finder reads | `engine/analysis.go` |
| Loudness readings for the transcript | `engine/audio.go` |

Only the last line of the render command changes: `-c:v libx264 -crf N`
becomes the system encoder and its own quality setting, because
`-crf` is an x264 idea and does not exist elsewhere. **This is done**, in
`engine/encode.go`. The encoder is chosen at render time from what this
ffmpeg actually has, macOS reaching for `h264_videotoolbox` and falling
back to `libx264`, with `--encoder` to name one instead. Preflight checks
that the chosen encoder exists rather than insisting on libx264.

Windows and Linux are deliberately not in that table yet. Windows has
`h264_mf` and Linux has VA-API or openh264, and a guess written there would
render every short made on that system. They go in when those builds are
first made and looked at.

**Checked against ffmpeg's own configure**, release 7.1, rather than taken
on trust, because the whole decision rests on it.

`EXTERNAL_LIBRARY_GPL_LIST` is the list of libraries whose use requires
`--enable-gpl`. In full: `avisynth`, `frei0r`, `libcdio`, `libdavs2`,
`libdvdnav`, `libdvdread`, `librubberband`, `libvidstab`, **`libx264`**,
`libx265`, `libxavs`, `libxavs2`, `libxvid`. **libass is not in it.** Its
filters ask only for libass itself: `ass_filter_deps="libass"` and
`subtitles_filter_deps="avformat avcodec libass"`.

Every filter the engine uses is outside the GPL set as well. Checked one by
one against `<name>_filter_deps` in configure: `crop`, `scale`, `pad`,
`afade`, `concat`, `subtitles`, `select`, `fps`, `format`, `setpts`,
`asetpts`, `aformat`, `setsar` and `showinfo`. A near miss worth knowing
about: `cropdetect` **is** GPL and `crop` is not, so a future filter picked
by name without checking is how this comes back. The GPL filters are things
like `delogo`, `eq`, `hqdn3d` and `nnedi`, none of which this engine wants.

So one thing is settled and **one thing is still to look at**: whether
`h264_videotoolbox` at a generous quality is visibly worse than
`libx264 -crf`. The product rule is that the picture stays as close to the
original as possible, so this gets looked at on a real render rather than
assumed. A short is twenty to thirty seconds, so the bitrate can afford to
be generous. `ffmpeg -L` on the finished build stays the final word on the
licence.

**What this does not fix.** H.264 is covered by patents, and Shorts and
Reels both want H.264, so that does not go away in any version of this. But
using the system's own encoder puts us where every commercial video app on
the Mac already stands, rather than shipping somebody else's encoder inside
a paid product. That is the difference this decision makes. It is a smaller
surface, not a guarantee, and none of this is legal advice.

## Why not the alternatives

**Use Apple's AVFoundation and drop ffmpeg on macOS.** The most native
answer, the smallest download, the fastest encode. It also means writing all
five jobs again in Objective-C, including rasterising the captions with Core
Text instead of libass, at which point a short rendered on a Mac and the
same short rendered on Linux no longer match. It cannot be compiled or run
in a cloud session either, so every round of it would be blind. It is a
project, not a packaging step. The one benefit that matters, encoding with
Apple's encoder, is what the decision above already gets.

**Keep the GPL ffmpeg and ship it as a separate process.** No engineering at
all. It is also what a lawyer would need to look at, which is the thing to
avoid.

**Write the pieces ourselves.** Muxing an MP4 is doable. Rasterising the
captions is doable, because the engine already decides where every word
goes. Decoding an arbitrary podcast video is not, and writing an H.264
encoder is not. Anything built here still bottoms out at a decoder and an
encoder, which is exactly where the difficulty was.

## The words, because three of them get mixed up

**Bundle** is a noun on macOS, not a verb. `framefairy.app` is a folder that
Finder draws as one icon, holding `Contents/MacOS/`,
`Contents/Frameworks/` and `Contents/Info.plist`. It is not an archive and
nothing unpacks it. It **is** the application.

That collides with the other sense already in this repository, where Vite
bundles the interface into `cmd/framefairy-app/dist/app/`. So, one name per
thing: **bundle** here means the `.app`, and what Vite does is **building
the interface**. **Bundler** is a JavaScript word and is not used for
shipping at all.

**Installer** is a program that puts things where they belong. macOS has
two shapes of it and we want the lighter one:

- **`.dmg`**, a disk image, which is barely an installer. It mounts, shows
  the `.app` beside a shortcut to Applications, and the user drags it
  across. That drag is the installation.
- **`.pkg`**, a real installer that runs scripts and asks for an
  administrator password. It is only needed to write outside the app's own
  folder, and we do not.

So we ship a `.dmg` holding one `.app`.

**Nothing is ever compiled on a customer's machine.** They have no
compiler. We build ffmpeg once, in advance, and the finished binary sits
inside the `.app` like any other file.

## What is where

| Stage | What exists | Where | Who does it |
| --- | --- | --- | --- |
| Source | the repository | GitHub | us |
| Build | `framefairy.app`, signed | a Mac or a CI runner | us, once per release |
| Download | `framefairy-1.0.dmg`, about 90 MB, notarised | a web page | the customer, one click |
| Install | `/Applications/framefairy.app` | their disk | they drag it |
| First run | the models arrive | their disk | the app, with consent |

| Piece | Who builds it | In the download | Ends up |
| --- | --- | --- | --- |
| The app, 19.5 MB, interface embedded | us, at build time | yes | `Contents/MacOS/` |
| sherpa-onnx and onnxruntime, 32 MB | k2-fsa, we copy and sign again | yes | `Contents/Frameworks/` |
| ffmpeg and ffprobe, about 40 MB | us, once, in advance, static, no libx264 | yes | `Contents/MacOS/` |
| Licence notices | us | yes | [THIRD_PARTY.md](THIRD_PARTY.md), shown in the app |
| The speech model, 490 MB | NVIDIA, we never touch it | no | `~/.framefairy/models/`, first run |
| A language model, 14.4 GB | Google, we never touch it | no | the same place, only if they choose local |
| `llama-server` | the user, if they choose local | no | wherever they install it |
| Settings and the episode list | the app | no | `~/Library/Application Support/` |
| An episode's work | the app | no | `<episode>.framefairy/`, beside the video |

## One build, used for development too

The rule is that what Tim runs every day and what a customer runs should be
the same thing. Today they are not, and four of the six differences are
exactly where shipping bugs live:

| | `make run` today | what a customer runs |
| --- | --- | --- |
| Engine and interface | the same code | the same code |
| ffmpeg | Homebrew's, GPL, with libx264, found on `PATH` | ours, LGPL, static, inside the app |
| Which encoder | **the same one, chosen from what that ffmpeg has** | the same |
| Which ffmpeg wins | **the one beside the program, then `PATH`** | the same |
| The speech library | from the Go module cache | from `Contents/Frameworks/` |
| Runs from | `bin/framefairy-app`, bare | `/Applications/framefairy.app` |
| `Info.plist` and privacy prompts | none, so none appear | present, so they appear |
| Signature | none | Developer ID, notarised |

Two of those rows are already the same, in `engine/encode.go` and
`engine/tools.go`. The encoder is chosen at render time from what the
ffmpeg in hand actually has, and the ffmpeg beside the program always beats
the one on the search path, so the day ffmpeg lands in the bundle it is the
one that runs without another change.

So `make run` builds and launches the `.app` rather than a bare binary.
Then the daily loop exercises the bundled ffmpeg, the relocated libraries,
the `Info.plist` and the privacy prompts, and the only thing a customer has
that Tim does not is the signature. A bug in any of it shows up on the day
it is made rather than on the day of a release.

## The blocker to clear first

The binary built today only runs on the machine that built it. sherpa-onnx
ships shared libraries, and its cgo directive writes the Go module cache
path into the executable as the place to find them:

```
RUNPATH  /root/go/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-linux@v1.13.8/lib/x86_64-unknown-linux-gnu
```

Nobody else has that folder, so the app dies before it draws anything.

**This is done**, in `scripts/carry-libs.sh`, and it runs on every build
rather than only when packaging, so what is run every day is what is
shipped. Proved by hiding the module cache and looking: a build made this
way resolves both libraries from its own folder, and the build as it was
yesterday answers `libsherpa-onnx-c-api.so => not found`, which is what a
customer would have seen. On Linux that needs `patchelf`, which the checks,
the cloud setup and CI now install.

What it does, per system:

| System | What it needs |
| --- | --- |
| macOS | the `.dylib`s in `Contents/Frameworks`, and `install_name_tool` pointing at `@executable_path/../Frameworks` |
| Linux | the `.so`s beside the binary and the runpath set to `$ORIGIN` |
| Windows | the DLLs beside the `.exe`, which `scripts/check.sh --copy-dlls` already knows how to do |

It has never been noticed because everyone who has run the app also built
it.

## Who builds the disk image

Two jobs, on two different clocks, and keeping them apart is the point.

**The ffmpeg job runs by hand, rarely.** Building ffmpeg from source takes
far longer than building the app and its answer changes only when we change
the configure line or take a new ffmpeg version. So it is its own workflow,
started by hand, and what it produces is a versioned archive kept as a
release asset: `ffmpeg-lgpl-7.1-macos-universal.tar.gz` and its checksum.
The build records its own configure line and the `ffmpeg -L` output beside
it, because that output is the proof the build is LGPL rather than our word
for it.

**The release job runs on a tag, on a macOS runner**, and downloads that
archive rather than building it. In order:

| Step | What happens |
| --- | --- |
| 1 | Build the interface, then the Go binary for arm64 and for x86_64, and `lipo` them into one |
| 2 | Assemble `framefairy.app`: `Info.plist`, the icon, the speech libraries into `Contents/Frameworks/` with their paths fixed, ffmpeg and ffprobe into `Contents/MacOS/` |
| 3 | Sign inside-out with the Developer ID: every library, then ffmpeg and ffprobe, then the app, with the hardened runtime |
| 4 | Make the `.dmg`, the `.app` beside a shortcut to Applications |
| 5 | Send it to Apple to notarise, wait, and staple the ticket to it |
| 6 | Attach the `.dmg` to the GitHub release |

Only step 5 needs anything secret, and only steps 3 and 5 need the Apple
Developer account. Everything up to step 2 runs on any machine, which means
the `.app` can be built and opened long before there is a certificate to
sign it with. That is what makes it possible to start now.

## How the building works

Wails v3 packages from a `build/` folder of per-platform Taskfiles, driven
by the `wails3` command. We have none of it: no `build/`, no `Taskfile.yml`,
and `make` calls `go build` directly. The templates in the pinned
v3.0.0-beta.23 cover an `.app` bundle and a universal binary for macOS, an
NSIS installer for Windows, and AppImage plus deb and rpm for Linux, and
there is notarisation support in the toolchain. Adopting that layout is the
cheap path, with one edit: their Windows task sets `CGO_ENABLED=0` and
sherpa-onnx needs 1.

**Cross-compiling is not possible.** cgo rules it out, through sherpa-onnx,
through `chrome_darwin.go` linking Cocoa, and through Wails itself. Every
platform needs its own machine, and macOS signing and notarising need a Mac
anyway. CI has runners for all three.

One thing each platform wants that is easy to forget:

- **macOS**: signing is inside-out. Every nested binary is signed first,
  each dylib and each bundled program, and the app around them last. See
  below.
- **Linux**: build against an old glibc, in a container or on the oldest
  supported Ubuntu, or the binary fails on anything older than the machine
  it was built on.

## Signing on macOS, and what entitlements are

An entitlement is a key and a value baked into the code signature. It is
not a file the app reads. The system enforces it and changing one means
signing again.

Two things share the word and only one of them is ours:

- **App Sandbox** confines an app to a container. It is required for the
  Mac App Store, and we are not going there.
- **The hardened runtime** is required for notarisation. This one is ours.

The part worth understanding: under the hardened runtime an entitlement is
mostly permission to **switch a protection back off**, not permission to do
something. Apple turns a set of defences on and each entitlement is an
exception that has to be justified.

| Protection | What it stops | What we need |
| --- | --- | --- |
| Library validation | loading a library signed by another team | nothing. We sign the speech libraries ourselves, which notarisation needs anyway |
| Unsigned executable memory, and JIT | memory that is writable and executable at once | nothing. Neither the app nor ffmpeg does it |
| `DYLD_*` variables | they are ignored | nothing. We do not use them |

**Starting another program is not one of the things it restricts.** A child
process is its own process with its own signature, so running our own
ffmpeg needs no entitlement, and neither does running the user's
`llama-server`, which lives outside the bundle entirely now that they
install it themselves.

### What building our own ffmpeg really costs

- **It is signed with our Developer ID**, before the app around it, and
  notarised with everything else. Mechanical.
- **It is built statically**, and this is the part that matters. A dynamic
  ffmpeg with libass drags in freetype, fribidi and harfbuzz as separate
  libraries. Each one then needs signing, and each one needs its search
  path pointed inside the bundle, which is the sherpa-onnx problem above
  four more times over. A static ffmpeg is one file with nothing to chase.

  Static is right on the licence side too. The ffmpeg command and the
  libraries inside it are one LGPL program, we ship it unmodified and we
  publish the source we built it from, which is the obligation we already
  took on. It would only get complicated if LGPL code were linked into
  **our** binary, and it is not. ffmpeg stays a separate process.
- **No entitlement.**

### What does bite, and it is not an entitlement

macOS privacy prompts. Episodes live in the user's Desktop, Documents or
Downloads. A file chosen through the open dialog is granted implicitly, but
the app remembers the path and reads it again on the next launch, and that
is the moment macOS asks. The words in that prompt come from purpose
strings in `Info.plist`, `NSDocumentsFolderUsageDescription` and its
siblings for Desktop, Downloads and removable volumes. Leave them out and
somebody who has just paid for the app gets a blank-sounding prompt about
it.

## Still open

- **Updates.** A paid desktop app needs a way to update itself, and that
  touches signing on every platform. Nothing is decided.
- **Signing costs.** Apple's Developer Program is 99 dollars a year. Windows
  code signing needs a certificate whose key lives on hardware or in a cloud
  signing service, so it is a subscription rather than a one-off.
- **Whether a customer build records training data**, and whether that is a
  setting. Phase 5.7.
- **The licence key.** Last of all, as it always was.
