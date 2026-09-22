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
  app walks them through getting it, which is `scripts/models.sh` turned
  into something with a window: pick the model that fits the memory, show
  the size before it starts, download with a checksum, resume a broken one,
  and say plainly what it will cost in disk and in memory.

Speech is not part of that choice. Transcription is always local, always
Parakeet, and the speech model is 490 MB, which is a first-run download
nobody has to think about.

**A consequence worth having.** Because no model ships, we never
redistribute one. The app fetches Gemma from its own home, the way
`make models` does today, so its licence is between the user and Google
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

**What this leaves to do.** Ship an ffmpeg we built ourselves, configured
without `--enable-gpl` and with libass, as a plain executable inside the
app bundle. Distributing an unmodified LGPL binary asks for two things: the
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
`-crf` is an x264 idea and does not exist elsewhere.

**Two things to confirm on the first real build**, rather than assume:

- that every filter the engine uses is in the non-GPL set. The render uses
  `crop`, `scale`, `pad`, `afade`, `concat` and `subtitles`, and the
  analysis uses `select`, `fps` and `format`. All of them look
  unconditional, and `ffmpeg -L` on the finished build will say LGPL or it
  will not.
- that `h264_videotoolbox` at a high enough bitrate is not visibly worse
  than `libx264 -crf`. The product rule is that the picture stays as close
  to the original as possible, so this gets looked at rather than assumed.
  Shorts are twenty to thirty seconds, so the bitrate can be generous.

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

## What travels in the bundle

| Part | Size | Note |
| --- | --- | --- |
| `framefairy-app` | 19.5 MB | interface embedded |
| sherpa-onnx and onnxruntime | 32 MB | the speech runtime, see below |
| ffmpeg and ffprobe, LGPL | about 40 MB | built by us, no libx264 |
| Licence notices | nothing | [THIRD_PARTY.md](THIRD_PARTY.md), shown in the app |

Downloaded on first run, with consent: the speech model at 490 MB, and a
language model only if the user asks for one.

## The blocker to clear first

The binary built today only runs on the machine that built it. sherpa-onnx
ships shared libraries, and its cgo directive writes the Go module cache
path into the executable as the place to find them:

```
RUNPATH  /root/go/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-linux@v1.13.8/lib/x86_64-unknown-linux-gnu
```

Nobody else has that folder, so the app dies before it draws anything. It
is mechanical to fix and it is step one:

| System | What it needs |
| --- | --- |
| macOS | the `.dylib`s in `Contents/Frameworks`, and `install_name_tool` pointing at `@executable_path/../Frameworks` |
| Linux | the `.so`s beside the binary and the runpath set to `$ORIGIN` |
| Windows | the DLLs beside the `.exe`, which `scripts/check.sh --copy-dlls` already knows how to do |

It has never been noticed because everyone who has run the app also built
it.

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

Two things each platform wants that are easy to forget:

- **macOS**: every bundled dylib and every bundled executable is signed
  too, not just the app, and the hardened runtime that notarisation
  requires is where spawning our own ffmpeg needs the right entitlements.
- **Linux**: build against an old glibc, in a container or on the oldest
  supported Ubuntu, or the binary fails on machines newer than nothing.

## Still open

- **Updates.** A paid desktop app needs a way to update itself, and that
  touches signing on every platform. Nothing is decided.
- **Signing costs.** Apple's Developer Program is 99 dollars a year. Windows
  code signing needs a certificate whose key lives on hardware or in a cloud
  signing service, so it is a subscription rather than a one-off.
- **Whether a customer build records training data**, and whether that is a
  setting. Phase 5.7.
- **The licence key.** Last of all, as it always was.
