# A build that does not start is undone

Builds on
[2026-11-18-an-update-downloads-only-what-changed.md](2026-11-18-an-update-downloads-only-what-changed.md).
Speaker Trail, whose updater is ported from this one, made the same
change in its record of 20 November 2026. This one takes it trimmed, as
said below.

In 30 words: a new build is put in place only once its program loads,
and the build before is kept until the new one says it started. One
that never says so is replaced by the build before.

## The problem

The user, 28 November 2026: "I think we should do all of the changes
that are worth taking and do all of them in one pull request here."
This is one of them.

Every build the user gets is now signed again with the Developer ID and
the hardened runtime. Under the hardened runtime macOS refuses to load a
library signed by another team, and ends the program before any of our
code runs. Wails' helper, which puts a build in place on Relaunch, puts
the build before back only when the new one cannot be opened at all,
and removes it the moment the new one runs. Our own step on quit keeps
nothing. So a build that opened and then crashed at once, or whose
window never came up, stays, and an app that cannot start cannot update
itself either. The way back is a new download of the app by hand.

## Five simulations

1. **Who may know about whom.** Leave the steps as they are, and let the
   release tool refuse what would not start, as `check` and `sign`
   already refuse a build that could watch input or is not signed
   throughout. Cost: only what the release tool can see is caught. A
   build that crashes on the user's macOS, or with the user's files,
   passes.
2. **What changes together.** The step that puts a build in place owns
   all of it, on quit and on Relaunch: it starts the new program once,
   told to end at once, so the system loads it and every library it
   links, keeps the build before, puts the new one in place and writes
   down that it is on trial. The new build says it started once its
   interface asks where updates stand. On Relaunch the step opens it and
   waits for that, a minute at most. Put in place on quit, nobody waits,
   so each start counts itself, and the third that follows two that did
   not say they started puts the build before back. Names that cross: a
   file `update-before/trial.json` in the app's folder, and one
   environment variable that asks for the load. Cost: Relaunch goes
   through our step instead of Wails' helper.
3. **Where state lives.** Keep the builds in a store of our own, every
   one the app ever ran, and start the newest that says it started.
   Cost: a launcher outside the app, which macOS would see as another
   app, and disk that grows on the customer's machine.
4. **What runs together.** A watchdog that runs beside the new build
   for its first minute and puts the build before back when it ends.
   Cost: a second process with nothing to watch most of the time, and
   on quit there is no new build running to watch.
5. **What a test has to build.** A test of 2 needs only folders, a fake
   program that loads or does not, and a launch that says it started or
   does not, all handed in. A test of 3 or 4 needs processes.

## What converges

- **The build before is the way back**, in 2, 3 and 4. Only 3 keeps more
  than one.
- **The new build says it started**, in 2, 3 and 4, and in 2 it says so
  through a file the step reads.
- **A load before the swap** catches what 1 catches and more, on the
  user's own machine, before anything moves.

Dropped: the release-side check of 1, which adds nothing the load does
not see, the store of 3 and the watchdog of 4.

## The decision

Simulation 2, trimmed from Speaker Trail's in two ways:

- **The app's files are not put back** with the build before. Speaker
  Trail copies its one store file, which an older build refuses when it
  is of a newer version. Frame Fairy's settings and library have no
  version an older build refuses: it reads them and drops only what it
  does not know, a setting new in the build that failed at most. Every
  video's work lies in its own folder, which a copy of the app's folder
  would not reach anyway.
- **A build that did not start can still be picked by hand.** It is
  written down in `update-failed.json`, and a check the app makes by
  itself does not take it again. The Updates page says The newest build
  did not start. Picking its channel again on the Updates page takes it
  all the same, because the person may know the reason is gone, and the
  next build of the channel is taken as usual.

```
the app            Relaunch, or quit with a build ready, starts the step
the step           waits for the app to be gone, loads the new program,
(install.go)       keeps the build before in update-before, puts the new
                   one in place, writes trial.json; on Relaunch opens it
                   and waits a minute for trial.json to go
the new build      counts its start in trial.json first thing; the third
(trial.go)         start after two that did not say so undoes; its first
                   question to updates says it started, and
                   update-before goes
updates.go         a check by itself does not take the build in
                   update-failed.json; a pick by hand does
```

## What follows

- Relaunch puts the build in place with the step on quit, not with
  Wails' helper. Wails' updater still downloads, checks the signature
  and unpacks.
- Each update keeps the build before in the app's folder until the new
  one says it started. It is moved there, not copied, where the two
  folders are on one disk, as they are on a usual Mac.
- A load costs every update the start of one program that ends before
  it opens anything. Not timed, reasoned from the code: it is less than
  the start of the app that follows it.
- It was wrong if a build that does not start still stays: the step's
  log in the temporary folder, `framefairy-install-*.log`, says what it
  did, and the app's log says when it undid a build.
