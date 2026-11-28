# Updates

How a paying customer gets the next version of Frame Fairy. This page is
the research behind batch 5.9 in [GUI-PLAN.md](GUI-PLAN.md), written before
any code, so the decisions at the end are made once. The packaging it
builds on is in [PACKAGING.md](PACKAGING.md).

## What an update is

An update replaces `Frame Fairy.app` as a whole, and nothing else. The app
checks whether there is a newer version, downloads it, checks that it is
really ours, and swaps it in the next time the app starts. Everything the
customer made stays where it is.

**It downloads only what changed.** Each build is also published as a
zip that stores its files as they are, and beside it the build publishes
its parts: that zip in order, as its few bytes of headers and each file
named by its SHA-256. Every file is also published on its own, packed
with zstd, as a piece named by that SHA-256, once, whichever build holds
it. The app puts the stored zip together from the files it already has
with the same bytes and the pieces it downloads for the rest, and Wails'
updater checks that zip against its own signature as it checks any. A
usual update is the app program alone, about 7 MB of the 67.5 the whole
zip once weighed. A file that changes in the app while the update is put
together is downloaded as its piece. When the pieces it lacks weigh more
than the stored zip packed with zstd, about 33 MB, or the parts do not
work, a piece is gone or a panic stops the putting together, the app
downloads that packed zip instead. The stored zip itself is never
uploaded. The deflated zip stays at its address, signed as before, for
the apps from before parts. See
[the decision record](decisions/2026-11-18-an-update-downloads-only-what-changed.md).

**And of a changed file, only the bytes that changed.** A change to the
code moves addresses in the app program, not its whole size. So each
build also publishes a patch of each changed file, bsdiff's, against the
same file of the builds an app is likely to run: every build the
channel list names, and the newest build of up to three channels that
left it, a pull request just merged above all. The parts name
each patch by the SHA-256 of the file it starts from. An app that holds
that file downloads the patch, a few hundred kilobytes in place of a 7
MB piece, and makes the file with code of its own, which is checked
against the file's SHA-256 like a piece. A patch that cannot make its
file costs only its piece. The patches go up before the entry, and a
build has at most 60, fewer once the release is nearly full. The app's
log says for each update how many files it kept, patched and fetched.
See [the decision record](decisions/2026-11-28-an-update-downloads-the-bytes-that-changed.md).

That is the same on every platform and in every app that updates itself.
The differences are in who hosts the file, who signs it, what the person
sees, and whether a licence decides who may have it.

## What can be updated and what cannot

| What | Where it lives | How it changes |
| --- | --- | --- |
| The program and the interface | `Contents/MacOS/framefairy-app`, the interface embedded in it | the update |
| ffmpeg, ffprobe, llama-server | `Contents/MacOS/` | the update. They are ours and inside the app, so a newer llama.cpp or ffmpeg ships the same way as a change to the interface |
| The speech libraries | `Contents/Frameworks/` | the update |
| `Info.plist`, the icon, the licence notices | the app | the update |
| The speech model and the language models | `~/.framefairy/models/` | not by the update. The app fetches them itself, as it does on the first run. A new version that wants another model says which, with its checksum, and fetches it with consent, the way it does today |
| Settings and the episode list | `~/Library/Application Support/` | never replaced. A new version reads what the old one wrote |
| An episode's work: transcript, clip sets, captions, renders | `<episode>.framefairy/`, beside the video | never touched by an update. A new version has to read every format an older one wrote. The training records already work this way: `PromptVersion` rises, each version is a superset of the one before within its format, and the export writes every record in the format it was made in |

And what an update cannot do:

- **Change the machine.** The app needs macOS on Apple silicon. A version
  that needs a newer macOS than the customer has must not be offered to
  them. The feed says, per version, which macOS it needs, and the check
  leaves out what cannot run.
- **Replace a running app.** The swap happens after the app has quit, by a
  small helper, and the new version starts in its place. So an update
  never interrupts work in hand: it waits for the app to quit, the same way
  Cmd+Q waits for a search.
- **Replace an app it cannot write to.** An app started from the disk
  image, one macOS has moved to a hidden place because it was never
  dragged into Applications (App Translocation), or one in a folder the
  person has no right to write. Each needs its own answer: say where to
  move it, or ask for an administrator's password.
- **Go back.** An update is a step forward. A bad release is fixed by the
  next one, not by stepping customers back, because a newer version may
  already have written files the older one cannot read.
- **Change what was sold.** Whatever the licence promised on the day of
  purchase holds for that customer.

## How it works behind the scenes

Every self-updating app in use today does the same five things.

1. **A feed.** A small file on a web server lists the newest version, what
   it needs, where to download it, its size, its checksum and its
   signature. Sparkle calls this an appcast, an RSS file. Tauri calls it
   `latest.json`. The app fetches it now and then.
2. **A comparison.** If the feed's version is newer than the running one,
   and this machine can run it, there is an update.
3. **The download**, to a staging folder, with progress.
4. **Two checks of trust.** The download is signed with a key only we
   hold, and the app carries the public half, built in, so a file that
   was swapped on the server or on the way is refused. And the new app
   inside it is signed with our Apple Developer ID and notarised, as the
   first download was, so macOS runs it without a warning. Two keys, two
   jobs: Apple's says who made it, ours says this update belongs to this
   app.
5. **The swap.** When the app quits, a helper moves the old app aside,
   puts the new one in its place and starts it. It throws the old one
   away once the new one says it started. If anything fails on the way,
   or the new one never says so, it puts the old one back.

**The update key must never be lost.** Every copy of the app in the world
carries its public half. Without the private half, no update can ever
reach those copies again, and every customer would have to download the
app by hand once more. It lives in the release workflow's secrets and in
one offline backup, and nowhere else.

## The ways it is done

### Sparkle

The standard for Mac apps sold outside the App Store, open source, in use
for twenty years. It does all five steps and more: delta updates, which
download only what changed, phased rollouts, where one group of customers
is offered a release a day before the next, channels such as beta, a
critical flag, an administrator prompt when Applications is not writable,
and the standard update window every Mac user has seen. It signs with
Ed25519, `generate_appcast` writes the feed and the signatures, and the
public key goes in `Info.plist`.

For us it has two costs. It is an Objective-C framework, so the Go app
would reach it through cgo and a small bridge, the way `chrome_darwin.go`
reaches the window. And its window is its own, drawn by AppKit, not the
app's interface. It is macOS only. Windows has WinSparkle, which speaks
the same feed.

### Wails' own updater

The Wails we pin ships an updater,
`pkg/updater`, reachable as `app.Updater`. Read in the module source:

- It is Go, on every platform, with no bridge.
- It reads its releases from any of four sources: a **Sparkle appcast**,
  unchanged, **GitHub Releases**, **Keygen**, a licensing service, or an
  endpoint of our own.
- It verifies a SHA-256 or SHA-512 digest and an Ed25519 or ECDSA
  signature against a public key set when the app is built. A release
  that carries a signature and meets no key is refused, not installed.
- It unpacks a `.zip` or `.tar.gz` holding exactly one `.app`, refuses
  paths that escape the archive, and unpacks it beside the app, so the
  swap is a rename on one disk. After the app quits, a helper checks the
  build against the app it replaces, keeps a backup, swaps the bundle,
  and restores the backup if the new one fails its check or does not
  start. The app quits only once the helper says it is ready.
- It can check on a timer, and it has channels.
- It comes with a window of its own, with release notes, progress and
  Install, Skip, Remind and Cancel. It can also run with no window at all,
  `WindowNone`, and report every step as an event, so the update can be
  drawn by our own interface, with our own beam and fill.

What it does not do, as far as the source shows: ask for an administrator's
password, update an app that macOS has translocated or that runs from a
disk image, or download only what changed. Because it unpacks beside the
app, an app that may not write to its own folder already fails at the
download, and the Updates page says to move it to the Applications
folder.

### The Mac App Store

Apple hosts, signs, sells and updates. The licence is the App Store
receipt, and there is nothing of ours to run. Apple takes 15 percent under
a million dollars a year, 30 above.

The price for this app is the sandbox. Every bundled program has to run
sandboxed too, ffmpeg and llama-server included. And the app writes its
work into `<episode>.framefairy/` beside the video, which a sandboxed app
may only do in a folder the person has granted it. Possible, and a project
of its own. A later second channel, not the first way.

### No updates in the app

A download page and an email. Customers stay on old versions, and every
fix waits for them to notice. Not a real option for a paid app.

### How other apps do it

Nearly every Mac app sold outside the App Store uses Sparkle. Apps built on
Electron use its `autoUpdater` over Squirrel, which will only install a
signed app. Apps built on Tauri use its updater plugin, which will not
work at all without a signature, and checks it against a public key built
into the app. Chrome and Microsoft run updaters of their own. The shape is
the same everywhere: a feed, a signed file, a swap on restart. The choice
is only who wrote the code that does it.

## Licences and updates

**Decided: a licence is bought once and gets every update, for ever.** No
yearly renewal, no paid major versions. If there is a reason to update the
app, everyone who bought it gets the update. The worst that can happen to
a customer is that updates stop. So the licence plays no part in the
update check: the feed is public, every copy of the app reads it, and the
licence key only decides whether the app runs.

What was weighed. What a customer paid for decides which updates they
get, and the common models for a paid Mac app are these:

- **Every update, for ever.** Simplest. No income from people who already
  bought.
- **A year of updates, and the version you have is yours to keep.** After
  the year, renew to keep getting updates, or stay on the last one. Sketch
  and Panic's Nova work this way.
- **Paid major versions.** 1.x free for owners of 1.0, 2.0 bought again.

Two places can enforce it:

1. **The feed is public and the app decides.** Anyone can fetch the feed
   and the file. The app compares the release's date with the end of the
   licence's update year and only offers what is covered. Cheapest to run:
   GitHub Releases on this repository, which is public, costs nothing.
2. **The download is gated.** The server only hands the file to a valid
   licence. Keygen does exactly this: with its `RESTRICT_ACCESS` or
   `MAINTAIN_ACCESS` strategy an expired licence keeps every release made
   before it expired and is refused the ones after. Wails' updater already
   speaks to it. It costs from 49 dollars a month.

**The source is public**, on GitHub. Anyone can build the app from it
today, so gating the download protects little that the licence key in the
app does not already protect. That is an argument for the first way, and a
question for the licence key, batch 5.8, rather than for updates.

Who sells the licence, and handles VAT across the EU, is the same decision
as the licence key: a merchant of record such as Paddle or Lemon Squeezy,
or Keygen with a payment provider. Lemon Squeezy's licence API only
answers online, so every check needs the network.

## What the person sees

What the common apps do, bent to [the interface rules](../CLAUDE.md#interface-rules):

- **The app checks by itself**, once when it starts and then once a day,
  in the background. Nothing is shown while it does. What the app can do
  by itself, it does.
- **An update available is a quiet mark**, in one place, never a box: the
  box over the workspace is only for what cannot be taken back, and an
  update can always be put off. Where the mark sits is a design question
  for the batch that builds it.
- **Clicking it says what is new**, from the release notes, with **Update**
  and **Later**. Update downloads with the fill every download in the app
  already wears, in the control it was started from.
- **It installs when the app quits**, or at once with **Relaunch** if the
  person asks. Never while a search, a render or a transcription runs.
- **Check for Updates** in the app menu, where every Mac app has it.
- **A setting** to turn the daily check off, for someone who wants to be
  asked.
- **An update that failed** says why in words, and the app carries on as
  the version it was.

## Releases

A release is a tag, and everything after the tag is a workflow.
[PACKAGING.md](PACKAGING.md#who-builds-the-disk-image) already has the
steps up to the disk image. Updates add three:

| Step | What happens |
| --- | --- |
| 1 to 5 | Build, assemble, sign inside-out, make the `.dmg`, notarise and staple, as in PACKAGING.md |
| 6 | Pack the same signed, notarised `.app` as a `.zip` that stores its files as they are, with its parts and a piece for each file, what an update downloads. The `.dmg` stays the first download |
| 7 | Sign the `.zip` with the update key, a secret of the workflow |
| 8 | Write the feed entry: version, release notes, the macOS it needs, size, checksum, signature, channel |
| 9 | Publish the `.dmg`, the `.zip` and the feed where the app looks |

- **Version numbers** are `1.2.3`, one number in one place, `engine.Version`,
  set from the tag by the workflow. A patch fixes, a minor adds, a major
  is what a paid upgrade would be, if there ever is one.
- **Channels**: stable for customers, beta for the user and anyone who asks,
  from the same workflow with a different tag.
- **Release notes** in plain words, written with the pull requests that
  make them, because they are what the person reads in the app.
- **A bad release** is taken out of the feed at once, so nobody else gets
  it, and fixed by the next version.
- **What it needs before it can be tried for real**: batch 5.5, signing
  and notarisation. An update cannot be tested end to end without an app
  macOS will open.

## Updates for pull requests

The user's idea: run the app once, and whenever a pull request gets a new
commit, update the running app to it from inside the app, the way a
customer will update to a release. No terminal, no checking out a branch,
no `make run` again.

It is worth doing, and doing first, for two reasons. It uses the update
path every day, long before any customer does, so whatever is wrong with
it shows up on the user's Mac and not on the customers'. And it is most of the release
workflow already: build, assemble, pack, sign the update, write the feed,
publish. Only Apple's signing and notarisation, the stable channel and the
licence come later.

How it would work:

- **Every push to a pull request builds the app**, in CI, on a macOS
  runner, the same workflow a release will use. It takes ffmpeg and
  llama-server from the tools archive rather than building them. The
  repository is public, so the runner costs nothing. It is built on the
  push itself, which finds the branch's open pull request, and not on
  the pull request's own event: GitHub runs no workflow for a pull
  request while it is in conflict with main, and a commit pushed to a
  pull request in that state got no build. A branch with no open pull request builds
  nothing, and opening a pull request builds nothing either, so a pull
  request is built from its first push after it was opened. When the
  opening built too, a Dependabot pull request, pushed and opened in the
  same second, was built twice and the second build stopped the first.
  The build is of the branch as it is, not merged with main, so a
  pull request in conflict can still be tested.
- **A commit gets one build.** One that is built already, or that only
  changes the docs against the build there is, gets none of its own.
  `needsBuild` in `cmd/framefairy-release/needs.go` decides it, for the
  build and for the list alike. It measures against what is built, not against the push before,
  so a push of the docs that stops the build of a push of code before it
  does not leave that code without a build.
- **Each pull request is a channel**, `pr-N` after its number N, and main is one too. All
  their builds are files of one pre-release, `dev`. A version reads like
  `0.3.0-prN.a1b2c3d`, where `a1b2c3d` is the commit it was built from,
  shortened the way git and GitHub shorten it. It used to end in the
  workflow's run number, which said nothing about what was in the build.
  Nothing orders builds by their version: the app installs whatever build
  its channel has when it is not the one running.
- **Only a development build sees them.** A build made for the user shows, next
  to Check for Updates, which channel it follows: main or one of the open
  pull requests, by number and title. A customer's build is made without
  that list and only ever reads the stable feed.
- **Switching is allowed to go sideways.** Moving from one pull request to
  another is not an upgrade by version number, so a development
  build installs whatever the chosen channel's newest build is, whether its
  number is higher or not. Wails' updater can be given a small source of
  our own that does exactly that.
- **The running build says which it is**, pull request and commit, in the
  About box, so a report from testing always names what was tested.
- **Pull requests from forks are never built for it.** Only branches of this
  repository, which only the user and Claude push to. A development build runs
  whatever a pull request contains, so nothing from outside may get in.
- **Its own key.** The development builds are signed with an update key of
  their own, a different one from the customer key. A leak of it reaches
  development builds and nothing else. See the keys below.

What it can start with and what it cannot:

- **It needs no notarisation.** A file the app downloads itself carries
  no quarantine mark, so macOS runs it without asking Gatekeeper. Each
  build is signed with the Developer ID all the same, see The Developer
  ID below, so macOS knows it as the same app after every update.
- **It takes a few minutes per push.** The build runs in CI after each
  commit, so an update is ready a few minutes after Claude pushes, not at
  once. The pull request says when it is ready: the status Ready to update
  to turns green on the commit that was built.
- **The app lives in one place.** It is installed once, to
  `/Applications`, and updates itself there. An administrator can write
  there without a password prompt, and the user's account is one. `make run`
  still works, for a change the user wants to build themselves.
- **Every build shares one set of settings and episodes.** A pull request
  that changes a file format can leave a file another build cannot read.
  That is rare, it is the same rule a release lives by, and it is the risk
  of switching back and forth between pull requests that are not yet
  merged. Nothing is lost that the newer build does not read again.

### The keys

An update key is a pair of numbers made together, the way an SSH key is:
a private half that signs and a public half that checks. Ed25519, the
same kind Sparkle and Wails' updater use. They are not bought and not
issued by anybody: they are made once, on the user's Mac, with one command.

| Key | Signs | Private half lives | Public half lives |
| --- | --- | --- | --- |
| The release key | what customers download | a GitHub secret only the release workflow can read, behind an environment that waits for the user's approval, and a backup in the user's password manager | in the repository, built into every customer build |
| The development key | the builds of `main` and of pull requests | `FRAMEFAIRY_UPDATE_KEY`, a secret of the GitHub environment `updates`, which only main can enter, and the password manager | in the repository, built into every development build |
| The development Developer ID | the programs of every build of `main` and of pull requests, for macOS, see The Developer ID | `FRAMEFAIRY_DEVELOPER_ID` and its password, secrets of the environment `updates`, and the password manager | Apple's certificate, in every signed program |

- **The private half never leaves those two places.** It is not in the
  repository, not in a chat, not on a cloud session's disk. The user makes the
  pair, pastes the private half into the repository's secrets on GitHub's
  settings page, and keeps a copy in their password manager.
- **No branch can read it.** A push runs the workflow file of the branch
  pushed, so whatever the build workflow is given, any branch can take,
  by rewriting that file. So the build, `builds.yml`, asks for no secret
  and no token that can write, and leaves only the zip. `publish.yml` signs it.
  GitHub runs that one from main whatever branch was built, and the key
  sits in an environment only main can enter. Which channel, commit and
  version a build is, it works out again from GitHub's record of the run,
  not from anything the build wrote. The build once signed itself
  with the key in its own environment, and a branch could also have
  replaced main's entry in the list.
- **The record of the run is checked too.** It names the branch, but a
  tag called `main`, pushed by a branch whose own `builds.yml` runs on
  tags, gives a record that reads like a push to main, at any commit. So
  `publish.yml` publishes only a push run of `.github/workflows/builds.yml`,
  and only a commit that main's history holds, or the history of the pull
  request's newest commit, by GitHub's compare, which
  `framefairy-release publish-plan` asks. main is compared by its commit, never by
  its name, which the tag takes. A commit main does not hold fails main's
  publish with an error that says to look at the tags.
  A commit older than the build its channel has, a run of it run again
  or a tag named `main` at an old commit of main, is not published
  either, so a build never takes a newer one's place.
  "No token that can write" is how main's `builds.yml` is written, not a
  wall: a branch's copy can ask for one. What holds is the environment,
  the signatures and this check.
- **The public half is not a secret.** It sits in the repository and is
  built into the app, which is what lets the app check a download without
  asking anybody.
- **A workflow started by a pull request from a fork gets no secrets**,
  which is GitHub's rule, and ours on top: fork pull requests are not
  built at all.
- **A development build does not trust a release, and a customer build
  does not trust a development build.** Each carries one public half. A
  customer can never be handed a pull request's build, even by mistake.
- **The Developer ID is a certificate of its own for development
  builds.** A customer release will be signed with a separate Developer
  ID certificate of the same team, behind the release environment, so a
  development one that leaks is revoked without touching what customers
  run.
- **Losing the release key** means no update can reach the copies already
  sold. Losing the development key costs nothing but a new pair and one
  install by hand. That is why only the release key needs the approval
  step.

### What is never signed

A build that could read input meant for other apps, watch the screen or
the camera, or ask for administrator rights, is refused before it is
signed, so no app is ever offered it. `framefairy-release check` reads every file of the zip
first, see `cmd/framefairy-release/inputs.go`, with Go's `debug/macho`, so
it needs no Mac. It runs in a step of its own, before the step that holds
the update key, because the zip is a branch's code that nobody has
reviewed yet, and `sign` runs only once it passed:

- **Every file.** Each file's bytes are read for the names below, a
  script or a licence as well as a program, since the app could run a
  script through `/bin/sh`. A file that starts like a program but that
  Go cannot read as one fails, because its imports, libraries and
  entitlements cannot be checked and macOS may still run it. A zip that
  says it unpacks to more than 4 GB fails before anything is read, since
  every file is read whole and at once. A build holds about 160 MB.

- **Imports and names.** No program imports, or names anywhere in its
  bytes, what taps every key, reads other apps' windows, records the
  screen or opens the camera: `CGEventTapCreate`, `IOHIDManagerOpen`,
  `AXIsProcessTrusted`, `CGWindowListCreateImage`, `SCStream`,
  `AVCaptureDevice` and the rest of the list. A name in the bytes catches
  a function looked up while the program runs, with `dlsym` or
  `NSClassFromString`.
- **Administrator rights.** No program imports or names what asks for an
  administrator's password or runs something as the root user:
  `AuthorizationExecuteWithPrivileges`, `AuthorizationCreate`,
  `AuthorizationCopyRights`, `SMJobBless`, a daemon of `SMAppService`,
  AppleScript's `with administrator privileges` and `/usr/bin/sudo`. No
  file of the build is marked set-user-ID or set-group-ID, which would
  run it as its owner. What looks alike stays: `setuid` and `setgid`,
  which Go's own `os/exec` imports and which only give rights up, and
  `SMAppService`'s `mainAppService`, with which Wails opens the app at
  login as the person.
- **Linked libraries.** Every program links what is on its own list and
  nothing more. A new framework, ScreenCaptureKit or AVFoundation, or a
  program the list does not know, fails.
- **Info.plist** gives reasons for the four folders a video is opened
  from and the microphone, for Dictate, and for nothing else.
- **Entitlements**: the microphone and nothing else. The build is signed
  ad hoc without any, and `harden` adds the microphone after the check,
  from main's `build/entitlements.plist`.

A refused build says so on the commit that was built, as a failed
Ready to update to with the first thing found, so the pull request shows
why its build never reached the Updates page. A publish that fails for
any other reason says so the same way.

The signer runs from main, so the lists are main's, and a branch that
changes them changes nothing about how its own build is judged. Each
change to a list is a line in a pull request that says why. The linters
warn earlier, in the pull request itself: forbidigo in `.golangci.yml`
holds that our Go code registers no global shortcut and reads no
clipboard through Wails, and ESLint in `frontend/eslint.config.js` that
the interface's scripts open no microphone, camera or screen and read no
clipboard by themselves. And a change to `scripts/bundle-macos.sh` runs the signer's own test of
the Info.plist it writes.

What it cannot catch is code that hides the name it looks up, built from
pieces while it runs. Then macOS is the wall: none of these inputs works
until the person allows it in System Settings. The check makes sure the
app never asks.

### The Developer ID

Every build the user updates to is signed again with their Developer ID
and Apple's hardened runtime, by `framefairy-release harden` in
`publish.yml`, see `cmd/framefairy-release/devid.go`. The hardened
runtime keeps other programs from putting their own code into the app:
macOS ignores the `DYLD_` settings it is started with, refuses debuggers,
and loads only libraries signed by Apple or by the same developer. The
app is signed as entitled to the microphone and nothing else,
`build/entitlements.plist`, because under the hardened runtime an app
without it hears only silence on Dictate. macOS still asks the first
time.

A Developer ID also makes every build the same app to macOS. A build
signed ad hoc looks like a new app after each update, so macOS asks
again for Documents and for the keys the app keeps in the keychain. The
first build signed with the Developer ID asks once more, then no update
does.

`codesign` and the keychain are only on a Mac, so the job that publishes
runs on one. The job `plan` asks on Linux first whether there is
anything to publish, so a run that publishes nothing takes no Mac. The
Mac runs main's signed build of the release tool for a Mac, which
`release-tool.yml` makes beside the one for Linux.

codesign puts Apple's time stamp into every signature, so a library
signed again is a new file even when its code did not change, and an
update would download ffmpeg, llama-server and the speech libraries
every time. So `framefairy-release signed-before` first downloads the
libraries and programs of main's build and of the channel's, as they
were signed then, read the way the app reads a build. `harden` keeps
such a copy in place of the one it just signed when the two are the same
signed code: the same bytes up to the signature, nothing after it, the
same blobs in the signature but the CMS one, which holds the time, and
the same certificates in the CMS. The rest of the CMS is not compared,
since `codesign --verify` checks it. A library signed before a
certificate was renewed is never kept. The kept copy is checked against
the team like every other, and the app is sealed around it. See
`cmd/framefairy-release/kept.go`.

Builds made on the Mac with `make` stay signed ad hoc, without the
hardened runtime, like Xcode's Sign to Run Locally. Under it an app
signed ad hoc may load no library but Apple's, and the speech libraries
are not Apple's.

Until `build/team-id.txt` names the team, builds go out signed ad hoc,
as they are made, and the run says so. Once it does, a build is
published only when it is signed with that team's Developer ID, and a
missing secret fails the run on main and on the commit, never quietly.
To set it up once:

1. In [Certificates](https://developer.apple.com/account/resources/certificates/add),
   choose **Developer ID Application**. Apple asks for a certificate
   request: on the Mac, in **Keychain Access**, choose **Keychain Access,
   Certificate Assistant, Request a Certificate From a Certificate
   Authority**, enter the email address, choose **Saved to disk**, and
   upload that file. Download the certificate and double-click it. It
   shows in Keychain Access under **My Certificates**, with its private
   key under it. Make one for this app's development builds alone, so it
   can be revoked without the certificates of other apps or of a
   customer release. Apple allows five.
2. In Keychain Access, right-click the certificate, **Export**, and save
   it as `developer-id.p12` on the Desktop with a new password. The file
   holds the private key too, which Apple cannot give out again, so it is
   the backup. Put it and its password in the password manager.
3. In Terminal, `base64 -i ~/Desktop/developer-id.p12 | pbcopy` puts the
   certificate on the clipboard as text.
4. On GitHub, in the environment `updates`, add two **environment
   secrets**: `FRAMEFAIRY_DEVELOPER_ID` with the clipboard, and
   `FRAMEFAIRY_DEVELOPER_ID_PASSWORD` with the password. Then delete
   `developer-id.p12` from the Desktop.
5. The team ID, ten letters and digits under **Membership details** in
   the [account](https://developer.apple.com/account), goes in
   `build/team-id.txt`. It is not a secret: every app the team signs
   carries it.

From then on `framefairy-release sign` reads the team from
`build/team-id.txt` itself and refuses a build in which
any program is signed ad hoc, without the hardened runtime, or by
another team, or whose app is signed as anything but
`com.framefairy.app`, or any other program or library as anything but
its part of it.

Every build of a pull request is signed with the Developer ID too, so
the user can try it. Its zip comes from the pull request's branch and is
unpacked on the Mac that holds the certificate, so `harden` trusts
nothing in it:

- It runs after `check`, unpacks the zip itself, and only files and
  folders inside `Frame Fairy.app`. A link, or a name that leads out of
  the .app, stops it before anything is written.
- It runs only `security`, `codesign` and `ditto`, from `/usr/bin`,
  never a program found through `PATH`.
- It reads both secrets once and takes them out of its environment, so
  no program it starts sees them. Every step that holds a key runs the
  release tool built before them, so `go` and its compiler never see
  either key. The key goes into a keychain made for the run, marked so it
  cannot be exported again, and the keychain is deleted at the end, also
  when signing fails.
- It takes Apple's certificate between the Developer ID and Apple's root
  only when it is signed by the Apple Root CA that macOS itself ships.
- It signs the app as `com.framefairy.app`, whatever its `Info.plist`
  says, and each program and library in it as a part of it, by its file
  name, like `com.framefairy.app.llama-server`. macOS gives code of the
  team the microphone and the keychain items of the app it is signed as,
  so no build, and nothing in one, can be signed as another app of the
  same team.
- It refuses a build with a program anywhere but `Contents/MacOS` and
  the libraries of `Contents/Frameworks`, which it signs itself.
- Once signed, codesign checks every program and the app against
  Apple's certificate of the team, not only the team the signature
  names, which is a string.

The job `outside` of `mac-nightly.yml` signs a bundle the same way, with
a certificate the test makes, and has the release tool read what
codesign wrote, see `hardened_real_darwin_test.go`.

If a build signed this way does not open, the app cannot update away
from it. `make install` in a checkout puts a build made on the Mac back
in `/Applications`, and Updates works again from there. Removing
`build/team-id.txt` on main sends builds out signed ad hoc again.

The certificate runs out on the date Keychain Access shows. A new one is
made with steps 1 to 4. Every signature carries Apple's time stamp, so a
build signed before then stays good after.

The builds are not notarised. Notarising is what lets a downloaded app
open past Gatekeeper on any Mac. The app downloads its updates itself,
and macOS asks Gatekeeper only of files a browser or mail marks as
downloaded, so nothing here needs it. The first download for customers
does, see [PACKAGING.md](PACKAGING.md).

### How the app knows the pull requests

The channel list is one small public file, a file of the release called
`dev` in this repository, kept at two fixed web addresses as two copies:

```
https://github.com/timmrkz/framefairy/releases/download/dev/channels-a.json
https://github.com/timmrkz/framefairy/releases/download/dev/channels-b.json
```

The address is built into the app. What is in the file is not: the build
workflow writes it again whenever a channel gets a new build, so a build
made today finds a pull request opened tomorrow.

GitHub cannot replace a release file in one step. `gh release upload
--clobber` deletes the old file before it uploads the new one, so for the
length of an upload there was no list, and an app that checked then was
answered 404 and said the check did not get through. The list is written
twice on every push to a pull request, so the user ran into it. The first fix
uploaded the new list under a name of its own and renamed it into place,
and the app waited out a 404 for seven seconds. The user still ran into it:
after the rename GitHub went on answering 404 at the list's address for
a few seconds more, measured at up to four from outside, and how long is
GitHub's to decide.

So the list is never replaced where an app reads it. There are two
copies, and `writeChannels` in `cmd/framefairy-release/channels.go`
replaces only the older one each
time, which leaves the newer one where it is the whole time. The app
reads both at once and keeps the one written last, by the time the list
says it was written, see `fetchCopies` in `updates/source.go`. The
workflow writes the list once at a time and each writing takes half a
minute, so the copy left alone is never one GitHub is still catching up
on. `channels.json` is still written, the old way, for the builds made
before the copies, and the app falls back on it, with the seven seconds
of waiting, when neither copy can be read. Any other answer is said at
once.

The app fetches it the way it fetches any file. It does not call GitHub's
API and has no idea what a pull request is. The file lists every channel
with its newest build, so one fetch is the whole check:

```json
{
  "channels": [
    {
      "channel": "pr-N",
      "name": "#N How the app updates itself",
      "version": "0.3.0-prN.a1b2c3d",
      "commit": "a1b2c3d4e5f6",
      "url": "https://github.com/timmrkz/framefairy/releases/download/dev/app-pr-N-0.3.0-prN.a1b2c3d.zip",
      "size": 187000000,
      "sha256": "…",
      "signature": "…",
      "claim": "…",
      "published": "2026-09-25T20:54:46Z",
      "newest": "6ceea6d1f2a3"
    }
  ]
}
```

- **The workflow keeps it true.** A push to a pull request builds it and
  refreshes its entry. `newest` is there while a pull request, or main,
  has a commit whose build has not come yet: the list is written again
  the moment a push starts, beside its build, and once more when the
  build is published, which takes it away. A publish that waits half an
  hour to enter the `updates` environment is cancelled, because it would
  hold its channel's queue for good, which kept main on one build for
  three days in October. The page then says **A newer commit is
  being built** and names it, rather than offering the build before as
  the newest. That build can still be installed. A commit that only
  changes the docs has no build coming and is never named. Neither is a
  commit whose build already ended without one: it failed, was cancelled
  or was refused by the signer, see `ended` in `cmd/framefairy-release/channels.go`. The list is
  written again whenever a build ends, however it ended, so the page
  stops saying a build is coming as soon as it is known that none is.
  Such a commit was once named until the next push. A pull request that is merged or closed has its
  entry taken away, and its builds go an hour later. Every writing of the
  list also takes away the entry of any pull request that is not open,
  because GitHub runs nothing for a pull request that no longer merges
  into main: a pull request was closed in that state, the close was never seen, and
  the app went on offering it.
- **The channel followed is read from its own entry too.** A build and
  its entry, `channel-<channel>.json`, are uploaded one after the other,
  and the list is written from the entries only after that, in one queue that
  every build of every channel shares. On 20 October the list took two
  minutes to follow a build of a pull request, and a pick in between came back at
  once as the build before, which was kept in the cache. So every
  read of the list also reads the entry of the channel followed, and
  when the entry is of a newer build, by `published`, it takes the
  list's place for that channel, with the list's name, `newest` and
  `conflict`. The entry is checked the way the list is, claim and all.
  One that is missing for a moment while it is replaced, or is of the
  build the list has, changes nothing. The zip goes up before its entry,
  because one upload of both sent them side by side, and the entry, a
  few hundred bytes, named a build for a minute before its zip could be
  downloaded. An app that read it then would have said the download
  failed. GitHub takes a few seconds to answer for a new file, see the
  list above, and the entry has that wait after the zip, not before it.
- **A pull request in conflict says so.** One that no longer merges into
  main keeps its entry, because its build is of the branch as it is and
  can still be tried, and the list marks it with `"conflict": true`. The
  Updates page then names it **PR #N, conflicts**,
  the way it names a closed one, in the list and on the trigger when it is
  followed. GitHub runs no CI on such a pull request, so the mark says its
  build was not tested. Followed, the list wears the colour of a warning,
  and the line under the build starts with the warning triangle and says
  CI has not tested it. While a build of it is looked for, downloads or
  waits for Relaunch, the line keeps its dot, in the colour of a warning. The workflow asks GitHub whether each pull request
  merges every time it writes the list, which is on every build, main's
  included, so a pull request main moved away from is marked once main's
  build is published. While GitHub has not worked it out yet, nothing is
  marked until the next writing. Like `newest`, it is not signed.
- **Nothing in it is trusted.** Anybody on the way could change the file.
  What makes a build safe is two signatures, made with the development key
  and checked against the public half built into the app. The
  `signature` is over the zip's checksum, which Wails' updater checks
  before anything is unpacked, so a list that points somewhere else can
  only point at a file that fails. The `claim` is over what the entry
  says the build is, its channel, version, commit, size and checksum
  together, and the app checks it before it downloads anything. Without
  it, any zip ever signed could be offered as any channel and any
  version: an old build of main as the newest, or a pull request's as
  main, by anybody who could write the list. With it, a build can only
  be offered as what it was signed as. The name and the time are left
  out: a pull request's title can change after its build, and neither
  decides what is installed. An entry from before claims were made stays
  in the list for the apps that do not check them, and an app that does
  will not install it until a new build of that channel brings one. See
  `ClaimMessage` in `updates/updates.go`.
- **No step back within a channel.** The claim says which build an entry
  is, not when it was made. So whoever can write the release could put
  back an older entry of a channel, signed as it was then, with a bug
  fixed since. Each entry carries `made`, when the run that built it was
  created, which a run made again keeps, and `order`, Ed25519 over the
  claim of the stored zip and that time together. The app keeps the
  newest `made` it took of each channel in `update-order.json` beside
  `updates.json`, and refuses a build of that channel made before it,
  see `Order` in `updates/order.go`. Picking another channel is never
  refused. The publish refuses an older commit too, by its git history,
  see `publishPlan`, so this is the same rule where the build is
  installed. An entry from before orders were signed is taken only
  while nothing of its channel was taken with one.
- **A customer build will have no channel list.** It will know the stable
  feed and nothing else.
- Asking GitHub's API from the app was the other way. It would put the
  API into the app, with its limit of 60 requests an hour for anyone who
  does not sign in. A plain file can move to any other host by changing
  one address.

### End to end

1. **Once, the key.** The user runs `make update-key` on their Mac. It puts the
   public half in `cmd/framefairy-app/update-key.txt` and the private half
   on the clipboard, never in a file. They paste the private half into the
   secrets of the GitHub environment `updates` as `FRAMEFAIRY_UPDATE_KEY`,
   with the environment's deployment branches set to main only, and into their
   password manager, and the public half into the pull request, where
   Claude commits it. Until then nothing is built to update to, and the
   app says it has no key.
2. **Once, the app.** `make install` builds the app on the user's Mac and copies
   it to `/Applications`. Built on the Mac, it carries no quarantine mark,
   so macOS opens it. From then on it is started like any other app, not
   from the terminal.
3. **The user picks a pull request** in the list on the Updates page. Updates is the last row of
   the sidebar, and it says which build is running. The app reads the
   channel list, downloads its build with the fill on the Check button,
   checks it against the key, and says it is ready. Updates on the rail
   gets a dot. The user clicks **Relaunch**, and the app comes back as that pull
   request. The sidebar says `0.3.0-prN.a1b2c3d`, and the page adds
   the commit, which opens on GitHub.
4. **Claude pushes to that pull request.** About a minute later the list
   says a newer commit is being built, and the page says so, naming it,
   the next time it looks. A few minutes later the workflow
   has built `0.3.0-prN.e4f5a6b`, signed it, and put it in the channel list.
   The app looks by itself the moment it starts and every ten minutes
   after, and every twenty seconds while a commit is being built, so the
   build is found soon after it lands. It downloads it quietly and puts
   the dot on Updates. One click,
   one restart. **Check** looks at once, and reads the channel list again
   first, whatever else is going on, so a pull request whose first build
   has just been published is on the list the moment Check is clicked. It
   used to wait for a check already running, a download among them.
5. **The user picks another pull request**, or main. The app downloads that channel's newest
   build, sideways, and says it is ready. A pick while another channel's
   build is downloading takes over at once, and the page shows the new
   channel from the moment of the pick. A pick is always looked for, also
   one that lands just as the check before is ending: it used to leave
   word that check had already stopped reading, and the page said
   Checking until the next look, ten minutes later. The download before goes on, out
   of sight, to the end: every build goes into the cache first,
   `~/Library/Caches/FrameFairy/builds`, under its SHA-256, and from there
   to the updater, so a download nobody waits for any more still arrives,
   see `updates/fetch.go`. It used to be stopped with the wait, and what
   it had was thrown away, so a look at another channel cost the whole
   download again. Whatever the download before says on the way, its
   progress or that it finished, is not shown: every pick starts a round
   of its own, and a check only speaks for the round it began in. Going
   back to a channel finds its build kept and ready in the time it takes
   to unpack, or still on its way, and the fill goes on from where it is.
   The fill only ever goes forward. Handing a build that has just arrived
   over from the cache is not more of the download, so it says only that
   the build is whole. It used to count from nothing again, and the user saw
   the fill run back towards empty just before the end, then jump to
   full.
   A download that brings nothing for a minute, `fetchQuiet`, is given
   up on and says so, a connection that went away without closing. It
   used to wait for the whole half hour a download may take, and every
   check after waited on it.
   A build is never downloaded twice at once. A kept build is used only
   while it still hashes to what the list says, and the updater checks it
   again, checksum and signature, as it checks a download.
6. **The pull request is merged.** Its channel goes from the list, and an app still on
   it downloads nothing. The page says **PR #N is closed** after the
   warning triangle, the list names it, closed, in the colour of a warning, and nothing
   downloads until another channel is picked. The build kept for it is
   removed the next time the list is read, and so is any build a newer
   push has replaced: nothing is kept that the list does not name. It
   used to follow main by itself, and the user, meaning to pick another pull
   request, watched main download without having asked for it.
   The page says closed whenever the list does, whatever the last check
   said: the channel being gone and the line under the build are one
   thing in the state, see `settle` in `cmd/framefairy-app/updates.go`.
   Reading the list for the page, without checking, once marked the pull
   request closed in the list while the line under it still said Not
   checked yet.

**A build that is ready is replaced by a newer one.** The app always
holds the newest build of the channel it follows. While a build waits for
a relaunch, the looks go on, and a newer commit's build downloads quietly
beside it. The page keeps saying **A newer build is ready**, with
Relaunch, and quitting puts the waiting build in place, for as long as
the newer one is on its way. Once the newer one has fully arrived and
passed every check the first one passed, checksum, signature and
unpacking, it takes the waiting one's place in one step, the page names
its commit, and the waiting one is removed. A newer build that fails, or
is cut off, changes nothing, and the next look tries again. This works
because every build is downloaded by an updater of its own, see
`download` in `cmd/framefairy-app/updates.go`: Wails' updater throws away
the build it holds before it downloads another, so the waiting build used
to be gone the moment a newer one began, and for good if that one failed.
A build that Relaunch or quitting has begun to put in place is never
removed, whatever arrives after it.

**Relaunch waits for the work running.** Quitting into the new build
would stop a search, a render or a download half way, so while one runs
or waits the button is greyed out, its title says what it waits for, and
the line under the build says Relaunch once the search is done. It comes
back by itself when the work ends. The Go side holds the same line:
`Restart` refuses while work runs, and from the moment it finds none, the
queue takes no new work, see `closeIfIdle` in
`cmd/framefairy-app/jobs.go`, so no search starts between the look and
the quit. Following another channel only downloads, so it stops nothing.
Quitting stops the work the way any quit does, after asking, and the
search says Stopped with Continue in the new build.

**Quitting with a build ready puts it in place**, the way Chrome does:
the next start is the new build, and nothing opens by itself in between.
Relaunch does the same at once and opens it. Both go through a step of
our own, `install.go`: the same program, started again with five
variables in its environment, which waits up to a minute for the app to
be gone, starts the new build's program once, told only to load and end,
moves the app aside into `update-before` in the app's folder, and moves
the new build into its place. When the new build does not go in whole,
the old app is put back. It only ever replaces a `.app`, only with a
build the updater unpacked, and never an app that is still running. When
it goes wrong, why is in `$TMPDIR/framefairy-install-<pid>.log`. When it
goes right, it leaves no log.

**A build that does not start is undone.** Under the hardened runtime
macOS ends a program whose library it refuses before any of our code
runs, so the step loads the new program first, and a build that does
not load never goes in. One that goes in is on trial, in
`update-before/trial.json`, until its window has asked where updates
stand, and then the build before goes. On Relaunch the step opens the
new build and waits a minute for that. When it does not come, the step
ends the new build, puts the build before back and opens it. Put in place
on quit, nobody waits, so each start counts itself, and the third start
after two that did not say they started puts the build before back.
The settings and the library stay as the new build left them. The build
is written down in `update-failed.json`, the app does not take it again
by itself, and the Updates page says The newest build did not start.
Check takes it all the same, and the next build of the channel goes in
as usual. See
[the decision record](decisions/2026-11-28-a-build-that-does-not-start-is-undone.md).

**Wails unpacks a build beside the app**, in a `wails-update-` folder in
`/Applications` when the app is there, so moving it into place is a
rename on one disk. A build that is ready and then replaced by a newer
one is removed at once.

**The app clears what updating left** when it starts, in the temporary
folder and in the folder beside the app. macOS clears the temporary
folder by itself only of what has not been touched for days, and the
folder beside the app not at all, so every app is meant to tidy up after
itself. Removed: a log of a step on quit that went wrong, after a week,
and after a day Wails' log of a Relaunch and a build Wails unpacked and
never used. Wails names those the same for every app made with it, so
only the ones about Frame Fairy go.

A build made by `make`, and `make install` and `make run` are, starts out
following nothing, whatever `updates.json` says. It is a build somebody
made on purpose, not one that came from a channel, so a channel picked by
an earlier build is not carried over to it: the user made one and found it
following the pull request an earlier build had picked. `updates.json`
stays as it is, for the builds from a channel that read it. Following
nothing, the build does not look, so every `make run` does not fetch a
build to replace itself with. The line under the build says **Built on
this Mac**, that it stays as it is until a channel is chosen to follow,
and offers no Check, since there is nothing to look for. Check for
Updates in the app menu reads the list and looks for nothing.

Following nothing is where such a build starts, not a choice: the list
offers only channels, main and the open pull requests, and a closed one
while it is still followed, and its trigger says **Choose a channel**
until one is picked. Picking one follows it fully, the way a build from a
channel does: it looks at once, downloads the channel's newest build by
itself, Relaunch puts it in, and until then it looks again every ten
minutes. Once a channel is picked, another channel is what follows it.

## What is built

| Part | Where | What it does |
| --- | --- | --- |
| The channel list and the source | `updates/` | reads and checks the list, picks the channel followed, and hands Wails' updater the build, its checksum and its signature. A build with parts is put together from the running app's own files, the patches of a file it holds an older copy of, `patch.go`, and the pieces it lacks, `assemble.go`, and downloaded as the stored zip packed with zstd when that fails or weighs less, `whole.go`. A build without parts is downloaded whole. `parts.go` holds what the parts are and how a build is packed in them. A channel that has gone is followed by nothing, never by main by itself. Every build downloads into the app's caches first and goes on to the end whoever still waits for it, so a channel picked again has its build at once or on its way, and whatever the list stops naming is removed each time it is read, unless it is still arriving |
| The download and the swap | Wails' `pkg/updater`, `cmd/framefairy-app/install.go` and `trial.go` | Wails' updater downloads, checks the checksum and the signature and unpacks the `.app`. Our own step puts it in place, on quit and on Relaunch alike, once its program loads, keeps the build before until the new one says it started, and puts it back when it never does |
| The app's side | `cmd/framefairy-app/updates.go` | whether this build can update at all and why not, the check the moment it starts and every ten minutes after for a build from a channel, every twenty seconds while the channel has a commit being built, a newer build in place of one that is ready once it is whole, the picked channel in `updates.json` beside the settings, the newest build taken of each channel in `update-order.json`, the build that last did not start in `update-failed.json`, which a check by itself does not take, when the last check ended, the restart into a new build, which waits for work in hand |
| The interface | Updates, the last row of the sidebar, and its own page | the row says which build is running and wears a dot whenever the page marks something, in the colour the page wears it in: work on a newer build, looking, downloading or a newer commit being built, pulses in the accent, a newer build ready stands still in the accent, a pull request followed that closed or that conflicts with main stands still in the colour of a warning, the dot of work on it pulsing in that colour too, and a check that did not get through, or a newest build that did not start, stands still in red. Up to date, a build made on the Mac and a build that does not update itself wear none. Both read the one state and the one mark, in `frontend/src/lib/updates.svelte.ts`, so the row never says less than the page or something else. Its title then says what the page says. The page is one card: the build and its commit, which opens on GitHub, and the list of channels, which names the channel and nothing more, Branch main or PR #N, says Choose a channel while a build made on the Mac follows none, opens from its right edge, and says in its title what it is for. Under it one line says where things stand, up to date and when it last looked, a newer build downloading with how far, or ready, with the one thing to do at its end: Check while a channel is followed, or Relaunch, Chrome's word for it, which restarts into the new build. The dot on the row only comes once the build is on disk, so Relaunch never waits. A check or a download that did not get through says why in words a person uses, from `updateProblem`, no connection to GitHub, GitHub busy, nothing found, a build that could not be verified or a disk that took nothing, in one short sentence, and asks to check the internet connection only when the error was the network's, a certificate the Mac does not trust among them, which is what a hotel's Wi-Fi or a company's proxy shows it. What the system said is under it apart, in a field at the height of a control on one line that scrolls sideways, with a button at its end that copies it for a report, and Try again stands on the same row as the field. Looking is shown for at least 1.4 seconds, on the page and on the row alike, because a check that finds nothing is over before anybody can read that it happened. Check for Updates in the app menu opens it |
| The key and the signing | `cmd/framefairy-release` | every decision the two workflows make, one command a step. `key` makes the pair, `build-plan` says what a push is a build of and whether it needs one, `publish-plan` what a finished build is published as and refuses a commit its channel's history does not hold, `pack` writes the build again with each file stored as it is, its parts, its pieces, its patches against the builds `publish-plan` named and the zip packed with zstd, `check` refuses a build that could reach past the app, before the key is there, `sign` signs a build `check` passed, and the stored zip `pack` wrote beside it, and refuses a key that is not the app's, `publish` uploads the pieces and patches the release lacks, the zip, the stored zip packed, its parts, then its entry, then marks the commit, `not-published` marks a commit whose publishing failed and says why, `channels` writes the channel list and leaves out a newest commit whose build ended without one, and `unstick` cancels a publish stuck before its environment. What they ask GitHub is in `github.go`, behind an interface the tests replace. A decision about a build goes here, never into a script, see [the decision record](decisions/2026-11-14-update-decisions-in-go.md) |
| The build | `.github/workflows/builds.yml` | builds main and every push to a branch with an open pull request of this repository on macOS, and leaves the zip as an artifact. It runs the branch's code, so main's copy asks for no secret and no token that can write, which a branch's copy could, so what holds is the `updates` environment and the history check in `publish.yml`. A commit built already, or one that only changes docs against the build there is, gets no build, decided by `needsBuild` in `cmd/framefairy-release` |
| The publishing | `.github/workflows/publish.yml` | runs from main whenever a build starts or ends. It works out which channel, commit and version a build is from GitHub's record of the run, packs the zip in parts, signs it and the stored zip, uploads the pieces and patches the release lacks, the zip, the stored zip packed, its parts and its entry to the `dev` release, takes away the zips, packed zips, parts, pieces and patches no entry reaches, keeping the parts and pieces of the builds a new build is patched against, writes the list at the start of a push and when its build is done, and marks a pull request's commit Ready to update to the moment its entry is up |
| The make targets | `make install`, `make update-key` | the app into `/Applications`, and the key |

The signature is Ed25519 over the SHA-256 of the zip, which is what Wails'
updater checks, the claim is Ed25519 over the entry's channel, version,
commit, size and checksum, and the order is Ed25519 over the claim and
when the build was made. The app checks the last two itself. Sparkle signs the file itself, so a Sparkle appcast and
this list could never have shared a signature, and the list is a small
JSON file of our own rather than an appcast.

Still to come, in the order they are needed:

- **Tried on the Mac.** That a downloaded build runs, that the swap
  works in `/Applications`, and what the step's log says, in
  `$TMPDIR/framefairy-install-<pid>.log`, when it does not.
- **macOS asks for Documents again after every update.** It remembers a
  folder it let an app into by the app's signature, and a build signed ad
  hoc is known only by the hash of its own program, so every new build
  is an app it has never seen. Signed with a real certificate, macOS
  knows the app by its bundle identifier and the certificate, and asks
  once for good. Developer ID, batch 5.5, ends it. Until then a new build
  asks once. The user accepted that for now.
- **Installing when the app quits, tried on the Mac.** Built, see below,
  and not yet seen working on the user's Mac.
- **Customers.** The stable channel, Apple's signing and notarisation, the
  release key, and a release build that knows no channel list.

## Open or closed source, and where releases live

**Decided for now: the code stays public, and the builds live in this
repository's releases.** What follows is why the question came up, and
what going private would take, for the day it is asked again.

A licence has to be worth paying for. If the code is open, anyone can
build the app, and the check that asks for a licence is a line anyone can
delete. Language models make that deletion easier every year. So the
question of where releases live is really the question of whether the
code is public.

**What is true today.** The repository is public and has no licence file.
With no licence, the code is all rights reserved: anyone may read it, and
nobody may use, change or pass it on. That stops a business. It does not
stop a person with a compiler. It has no forks.

**What no choice changes.** A check can be removed from any app. Closed
code only raises the bar from deleting a line to patching a binary, and
language models lower that bar too. What makes people pay is that paying
is easier than not: a signed, notarised app that updates itself, against
a toolchain, a half hour building ffmpeg, and an app macOS warns about.
The licence key gates what matters most and costs least to leave open
elsewhere: a render without a watermark. That is batch 5.8.

The two ways, side by side:

| | Code public, source available | Code private, releases public |
| --- | --- | --- |
| Where the code is | this repository, public, with a licence that allows reading and building for yourself and nothing else, the way Aseprite does it | this repository, made private |
| Where releases are | this repository's releases | a second, public repository, `framefairy-releases`, holding only builds, feeds, the channel list and the source of the ffmpeg we ship, which its licence asks us to publish |
| Removing the licence check | delete a line and build | patch a binary |
| CI | free, as it is today, macOS included | 2000 minutes a month free, and a minute of macOS counts as ten. Past that, 0.062 dollars a macOS minute |
| Claude's cloud sessions | as today | as today. They work in private repositories |

**What private costs in CI.** A push that touches Go runs 10 to 15
minutes on macOS today, tests and fuzzing, and a build per push for the
pull request channel adds about 5 more. At twenty such pushes a day that
is around 400 macOS minutes, 25 dollars a day, 500 to 700 a month. Three
ways to bring it down, which can be combined:

- **A runner on the user's Mac.** GitHub runs the macOS jobs on a machine of
  our own, the M2 Max, faster than GitHub's and without the minutes. It
  works while the Mac is awake, and it only ever runs this private
  repository's own jobs.
- **Fewer macOS jobs.** Tests and fuzzing on Linux for every push, macOS
  only for what only macOS can show: the warning-free build and the app.
- **Builds on request.** A pull request is built for its channel when it
  is marked for testing, not on every push.

**Pull request builds are public in both ways**, because the releases
repository is. So every build, development or release, asks for the
licence key before it renders without a watermark. The user has a key like any
customer. A build is only ever as free as a release.

## Decisions

1. **The update policy**: decided. A licence gets every update for ever,
   and the licence plays no part in the update check.
2. **The mechanism**: decided. Wails' updater, drawn by our own interface,
   with a channel list of our own. It fits a Go app that runs on three
   platforms and draws its own interface. Before it is final for
   customers it has to prove itself on a Mac: a signed, notarised bundle
   swapped, an app in a folder the person cannot write, and one never
   moved out of the disk image.
3. **Open or closed source**: decided for now. Public, with the builds in
   this repository's releases.
4. **The channels**: open. Stable and beta for customers, or stable alone
   at first.
5. **Updates for pull requests first**: decided and built, see
   [What is built](#what-is-built).
