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
| An episode's work: transcript, clip sets, captions, renders | `<episode>.framefairy/`, beside the video | never touched by an update. A new version has to read every format an older one wrote. The training records already work this way: `PromptVersion` rises, and each version is a superset of the one before |

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
   puts the new one in its place, starts it, and throws the old one away.
   If anything fails on the way, it puts the old one back.

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

The version of Wails we pin, v3.0.0-beta.23, ships an updater,
`pkg/updater`, reachable as `app.Updater`. Read in the module source:

- It is Go, on every platform, with no bridge.
- It reads its releases from any of four sources: a **Sparkle appcast**,
  unchanged, **GitHub Releases**, **Keygen**, a licensing service, or an
  endpoint of our own.
- It verifies a SHA-256 or SHA-512 digest and an Ed25519 or ECDSA
  signature against a public key set when the app is built. A release
  that carries a signature and meets no key is refused, not installed.
- It unpacks a `.zip` or `.tar.gz` holding exactly one `.app`, refuses
  paths that escape the archive, swaps the bundle with a helper after the
  app quits, keeps a backup, and restores it if the new one fails to
  start.
- It can check on a timer, and it has channels.
- It comes with a window of its own, with release notes, progress and
  Install, Skip, Remind and Cancel. It can also run with no window at all,
  `WindowNone`, and report every step as an event, so the update can be
  drawn by our own interface, with our own beam and fill.

What it does not do, as far as the source shows: ask for an administrator's
password, notice an app that macOS has translocated, or download only what
changed. And it is part of a beta. Every one of those is a test on a real
Mac with a signed, notarised build before anything is decided for good.

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
- **It installs when the app quits**, or at once with **Restart** if the
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
| 6 | Pack the same signed, notarised `.app` as a `.zip`, the file an update downloads. The `.dmg` stays the first download |
| 7 | Sign the `.zip` with the update key, a secret of the workflow |
| 8 | Write the feed entry: version, release notes, the macOS it needs, size, checksum, signature, channel |
| 9 | Publish the `.dmg`, the `.zip` and the feed where the app looks |

- **Version numbers** are `1.2.3`, one number in one place, `engine.Version`,
  set from the tag by the workflow. A patch fixes, a minor adds, a major
  is what a paid upgrade would be, if there ever is one.
- **Channels**: stable for customers, beta for Tim and anyone who asks,
  from the same workflow with a different tag.
- **Release notes** in plain words, written with the pull requests that
  make them, because they are what the person reads in the app.
- **A bad release** is taken out of the feed at once, so nobody else gets
  it, and fixed by the next version.
- **What it needs before it can be tried for real**: batch 5.5, signing
  and notarisation. An update cannot be tested end to end without an app
  macOS will open.

## Updates for pull requests

Tim's idea: run the app once, and whenever a pull request gets a new
commit, update the running app to it from inside the app, the way a
customer will update to a release. No terminal, no checking out a branch,
no `make run` again.

It is worth doing, and doing first, for two reasons. It uses the update
path every day, long before any customer does, so whatever is wrong with
it shows up on Tim's Mac and not on theirs. And it is most of the release
workflow already: build, assemble, pack, sign the update, write the feed,
publish. Only Apple's signing and notarisation, the stable channel and the
licence come later.

How it would work:

- **Every push to a pull request builds the app**, in CI, on a macOS
  runner, the same workflow a release will use. It takes ffmpeg and
  llama-server from the tools archive rather than building them. The
  repository is public, so the runner costs nothing.
- **Each pull request is a channel**, `pr-18`, and main is one too. All
  their builds are files of one pre-release, `dev`. A version reads like
  `0.3.0-pr18.7`, where 7 is the workflow's run number, so a newer commit
  is a newer version.
- **Only a development build sees them.** A build made for Tim shows, next
  to Check for Updates, which channel it follows: main or one of the open
  pull requests, by number and title. A customer's build is made without
  that list and only ever reads the stable feed.
- **Switching is allowed to go sideways.** Moving from pull request 18 to
  pull request 20 is not an upgrade by version number, so a development
  build installs whatever the chosen channel's newest build is, whether its
  number is higher or not. Wails' updater can be given a small source of
  our own that does exactly that.
- **The running build says which it is**, pull request and commit, in the
  About box, so a report from testing always names what was tested.
- **Pull requests from forks are never built for it.** Only branches of this
  repository, which only Tim and Claude push to. A development build runs
  whatever a pull request contains, so nothing from outside may get in.
- **Its own key.** The development builds are signed with an update key of
  their own, a different one from the customer key. A leak of it reaches
  development builds and nothing else. See the keys below.

What it can start with and what it cannot:

- **It needs no Apple certificate.** Until batch 5.5 the builds are signed
  the way `make app` signs them today, ad hoc. A file the app downloads
  itself carries no quarantine mark, so macOS runs it without asking, the
  same as a build made by `make`. This has to be confirmed on Tim's Mac, and
  it is the first thing the batch that builds this tries.
- **It takes a few minutes per push.** The build runs in CI after each
  commit, so an update is ready a few minutes after Claude pushes, not at
  once. The pull request says when it is ready: the check called list, of
  Builds to update to, turns green.
- **The app lives in one place.** It is installed once, to
  `/Applications`, and updates itself there. An administrator can write
  there without a password prompt, and Tim's account is one. `make run`
  still works, for a change Tim wants to build himself.
- **Every build shares one set of settings and episodes.** A pull request
  that changes a file format can leave a file another build cannot read.
  That is rare, it is the same rule a release lives by, and it is the risk
  of switching back and forth between pull requests that are not yet
  merged. Nothing is lost that the newer build does not read again.

### The keys

An update key is a pair of numbers made together, the way an SSH key is:
a private half that signs and a public half that checks. Ed25519, the
same kind Sparkle and Wails' updater use. They are not bought and not
issued by anybody: they are made once, on Tim's Mac, with one command.

| Key | Signs | Private half lives | Public half lives |
| --- | --- | --- | --- |
| The release key | what customers download | a GitHub secret only the release workflow can read, behind an environment that waits for Tim's approval, and a backup in Tim's password manager | in the repository, built into every customer build |
| The development key | the builds of `main` and of pull requests | a GitHub secret the build workflow can read, `FRAMEFAIRY_UPDATE_KEY`, and the password manager | in the repository, built into every development build |

- **The private half never leaves those two places.** It is not in the
  repository, not in a chat, not on a cloud session's disk. Tim makes the
  pair, pastes the private half into the repository's secrets on GitHub's
  settings page, and keeps a copy in his password manager.
- **The public half is not a secret.** It sits in the repository and is
  built into the app, which is what lets the app check a download without
  asking anybody.
- **A workflow started by a pull request from a fork gets no secrets**,
  which is GitHub's rule, and ours on top: fork pull requests are not
  built at all.
- **A development build does not trust a release, and a customer build
  does not trust a development build.** Each carries one public half. A
  customer can never be handed a pull request's build, even by mistake.
- **Losing the release key** means no update can reach the copies already
  sold. Losing the development key costs nothing but a new pair and one
  install by hand. That is why only the release key needs the approval
  step.

### How the app knows the pull requests

The channel list is one small public file at one fixed web address, a
file of the release called `dev` in this repository:

```
https://github.com/timmrkz/framefairy/releases/download/dev/channels.json
```

The address is built into the app. What is in the file is not: the build
workflow writes it again whenever a channel gets a new build, so a build
made today finds a pull request opened tomorrow.

The app fetches it the way it fetches any file. It does not call GitHub's
API and has no idea what a pull request is. The file lists every channel
with its newest build, so one fetch is the whole check:

```json
{
  "channels": [
    {
      "channel": "pr-18",
      "name": "#18 How the app updates itself",
      "version": "0.3.0-pr18.51",
      "commit": "a1b2c3d4e5f6",
      "url": "https://github.com/timmrkz/framefairy/releases/download/dev/app-pr-18-0.3.0-pr18.51.zip",
      "size": 187000000,
      "sha256": "…",
      "signature": "…",
      "published": "2026-09-25T20:54:46Z"
    }
  ]
}
```

- **The workflow keeps it true.** A push to a pull request builds it and
  refreshes its entry. A pull request that is merged or closed has its
  entry taken away, and its builds go an hour later.
- **Nothing in it is trusted.** Anybody on the way could change the file.
  What makes a build safe is the signature over the zip's checksum, made
  with the development key and checked against the public half built
  into the app, before anything is unpacked. A list that points somewhere
  else can only point at a file that fails.
- **A customer build will have no channel list.** It will know the stable
  feed and nothing else.
- Asking GitHub's API from the app was the other way. It would put the
  API into the app, with its limit of 60 requests an hour for anyone who
  does not sign in. A plain file can move to any other host by changing
  one address.

### End to end

1. **Once, the key.** Tim runs `make update-key` on his Mac. It puts the
   public half in `cmd/framefairy-app/update-key.txt` and the private half
   on the clipboard, never in a file. He pastes the private half into the
   repository's secrets on GitHub as `FRAMEFAIRY_UPDATE_KEY` and into his
   password manager, and the public half into the pull request, where
   Claude commits it. Until then nothing is built to update to, and the
   app says it has no key.
2. **Once, the app.** `make install` builds the app on Tim's Mac and copies
   it to `/Applications`. Built on the Mac, it carries no quarantine mark,
   so macOS opens it. From then on it is started like any other app, not
   from the terminal.
3. **Tim picks #18** under Settings, Updates, Follows. The app reads the
   channel list, downloads #18's build with the fill on the Check button,
   checks it against the key, and says it is ready. Settings on the rail
   gets a dot. Tim clicks **Restart**, and the app comes back as pull
   request 18. Settings, Updates says which build it is, with the commit.
4. **Claude pushes to pull request 18.** A few minutes later the workflow
   has built `0.3.0-pr18.52`, signed it, and put it in the channel list.
   The app looks by itself every ten minutes, downloads it quietly and
   puts the dot on Settings. One click, one restart. Check looks at once.
5. **Tim picks #20**, or main. The app downloads that channel's newest
   build, sideways, and says it is ready.
6. **#18 is merged.** Its channel goes from the list, and an app still on
   it follows main, and says so.

A build made by `make`, and `make run` is one, follows nothing until a
channel is picked, and looks only when it is picked or Check is clicked.
Otherwise every `make run` would fetch a build to replace itself with.

## What is built

| Part | Where | What it does |
| --- | --- | --- |
| The channel list and the source | `updates/` | reads and checks the list, picks the channel followed, and hands Wails' updater the build, its checksum and its signature. Falls back to main when a pull request has gone |
| The swap | Wails' `pkg/updater` | downloads, checks the checksum and the signature, unpacks the `.app`, and after the restart swaps it in with a backup |
| The app's side | `cmd/framefairy-app/updates.go` | whether this build can update at all and why not, the check every ten minutes for a build from a channel, the picked channel in `updates.json` beside the settings, Restart that waits for work in hand |
| The interface | Settings, Updates, and the dot on Settings in the rail | the build running, Follows, Check with the beam and the fill, Restart. Check for Updates is in the app menu too |
| The key and the signing | `cmd/framefairy-release` | `key` makes the pair, `sign` signs a build and refuses a key that is not the app's, `list` writes the channel list |
| The workflow | `.github/workflows/builds.yml` | builds main and every pull request of this repository on macOS, signs, publishes to the `dev` release and writes the list. A push that only changes docs gets no build |
| The make targets | `make install`, `make update-key` | the app into `/Applications`, and the key |

The signature is Ed25519 over the SHA-256 of the zip, which is what Wails'
updater checks. Sparkle signs the file itself, so a Sparkle appcast and
this list could never have shared a signature, and the list is a small
JSON file of our own rather than an appcast.

Still to come, in the order they are needed:

- **Tried on the Mac.** That a downloaded build that is signed ad hoc
  runs, that the swap works in `/Applications`, and what the helper's
  log says, in `$TMPDIR/wails-update-<pid>.log`, when it does not.
- **Installing when the app quits.** Today a build that is ready waits for
  Restart. Quitting throws it away, and the next start downloads it again.
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

- **A runner on Tim's Mac.** GitHub runs the macOS jobs on a machine of
  our own, the M2 Max, faster than GitHub's and without the minutes. It
  works while the Mac is awake, and it only ever runs this private
  repository's own jobs.
- **Fewer macOS jobs.** Tests and fuzzing on Linux for every push, macOS
  only for what only macOS can show: the warning-free build and the app.
- **Builds on request.** A pull request is built for its channel when it
  is marked for testing, not on every push.

**Pull request builds are public in both ways**, because the releases
repository is. So every build, development or release, asks for the
licence key before it renders without a watermark. Tim has a key like any
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
