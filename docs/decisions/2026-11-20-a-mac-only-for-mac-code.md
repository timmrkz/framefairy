# A Mac only for Mac code

Issue #963. Builds on
[2026-11-08-one-make-target-per-ci-job.md](2026-11-08-one-make-target-per-ci-job.md),
which it changes in one place: a pull request runs the Go tests on a Mac
only when it changes code only a Mac builds, and what only a Mac can tell
otherwise runs every night.

## The problem

Tim, 20 November: "sometimes it appears that we have a lot of resource
congestion", then "consolidate the Mac-related workflows so that we can
run CI faster with fewer resources constraining other builds", and then:
"there may be checks running in those Mac OS CI jobs that don't require
those underlying system primitives at all".

The account runs five Mac jobs at once, for every branch together. Every
push to a pull request and to main took three:

- `macos` in `ci.yml`, `make unit` on a Mac, every Go test there is.
- `outside` in `ci.yml`, `make outside WERROR=1`, which builds the app
  and opens it from outside.
- `build` in `builds.yml`, `make app WERROR=1`, the app for the Updates
  page.

Read from the jobs of #962's last commit, 142b06a, one run each, they took
230, 134 and 116 s, 480 s of Mac time a push. From the runs of CI on main,
14 to 20 November, a Mac job of an outdated run waited 2491 s in the
queue in all, and `outside` of run 3330 never got a Mac in 34 minutes.

What in them needs a Mac:

- `macos` ran 2399 tests in 158 s. The slowest, 60 s, tests the job
  queue and is the same on every system. Only a few dozen tests run code
  only a Mac runs: the keychain, the free memory, the window's refresh,
  the hang dump's stacks, the frames decoder's chip. CLAUDE.md gave
  VideoToolbox as the reason for the job, but no test has reached it
  since the media engine became an interface.
- `outside` does need a Mac: macOS hands a `framefairy://` link to the
  app through Launch Services, Unlock writes the key to the keychain,
  and the app is quit and started again as a bundle. But the code that
  touches any of that changes in few pull requests.
- `build` needs a Mac for the app Tim updates to, and is the one place a
  Mac compiles the app with every warning an error.

## Five simulations

1. **Who may know about whom.** The compiler already knows which code
   only a Mac builds: a file whose build constraint names `darwin`. CI
   needs to know only the names of the files a pull request changed. So
   the code that behaves differently on a Mac is kept in files named for
   it, `*_darwin.go` and `*_darwin_test.go`, a check holds every file to
   that, and a filter on those names starts the Go tests on a Mac. The
   rest, the disk and the ffmpeg we build for the Mac, which no file says
   is the Mac's, is checked every night with the app opened from outside.
2. **What changes together.** The app the Updates page offers and the
   app opened from outside are one build, so one job builds both: the
   build of `builds.yml` runs `make app`, uploads the zip and then `make
   outside`, which finds every package compiled and only links the
   programs again. `ci.yml` loses `outside` and keeps `macos`. This was
   the first round of this pull request, built and reviewed. Two Macs a
   push, and no rule to keep. But every build to update to ends about
   33 s later, measured on one run, and one whose checks from outside
   fail is not offered on the Updates page until they pass. The full
   suite still takes a Mac on every Go push, though only a few dozen of
   its 2399 tests run code only a Mac runs. And Tim asked, at 20:01, why
   the checks from outside run on every push at all, when most changes
   touch nothing they check.
3. **Where state lives.** Which files are the Mac's is worked out where
   it is true, by the compiler: a Go program in the job `changes` asks
   `go list` with `GOOS=darwin` and `GOOS=linux` which files of the
   change build only on a Mac. Exact without a name, but a program in the
   way of every pull request, which the file names make unneeded once a
   check holds them, and a branch on `runtime.GOOS` is still invisible to
   it.
4. **What runs together.** Every Mac test runs every night on main, and
   on any branch by hand, and no push takes a Mac but the build. No rule
   to keep, but a change to the Mac's own code, the keychain or the
   window, reaches main untested, though it is the code most likely to
   break there. Tim asked for build constraints instead.
5. **What a test has to build.** The Mac's code moves into packages of
   its own, so the Mac job tests only those, in seconds. It moves about
   ten files out of the engine and the app, deep in both, and the job
   still needs the whole suite for what no file says is the Mac's.

## What converges

3 reaches the same filter as 1 with a program the check makes unneeded.
2 halves the Macs a push takes but keeps both kinds of Mac work on every
push, and makes the Updates page wait on the checks from outside. 1
takes the full suite off every push that does not touch the Mac's code,
and 4 takes the checks from outside off every push. 4 is kept for what no code says is the Mac's:
the full suite on a Mac and the app opened from outside run every night.
5 is the refactor 1 makes unnecessary.

## The decision

```
a pull request    the Go tests on a Mac only when a *_darwin*.go file, go.mod
                  or go.sum changed, ci.yml macos, filter mac of the job
                  changes
                  the build to update to, builds.yml build, as before
internal/maccode  a file only a Mac builds is named *_darwin.go or
                  *_darwin_test.go, and no other file uses runtime.GOOS
                  but in the ways the check allows
every night       mac-nightly.yml on main: make unit and make outside on a
                  Mac, two jobs, and Run workflow on any branch
```

The check is two tests in `internal/maccode`, which run in `make unit` on
Linux like every other test, so a pull request that breaks the rule fails
on Linux, with the file and line and what to do. It lists the uses of
`runtime.GOOS` it allows, not the ones it forbids, so a way of asking
nobody thought of fails until it is judged. A question a Mac and Linux
answer alike, `runtime.GOOS == "windows"`, or a switch that sends both to
one case, is no Mac code. Nor is `runtime.GOOS` handed on as a value, to
a function or method of this module, a field or a return, like
`SystemMedia(runtime.GOOS)`: the function takes the system as a
parameter, and a test on Linux hands it `"darwin"`. That is the one gap
the rule leaves on purpose: a change to the Mac arm of such a function
starts no Mac test, and its test on Linux is what holds it. Kept in a
variable, compared with a constant or handed to another package's
function, like `strings.EqualFold`, it is found.

Code only a Mac builds also comes from the modules: the speech library
for the Mac builds only there, and so do Wails' and `x/sys`'s darwin
files. A change to `go.mod` or `go.sum` starts the Go tests on a Mac too.

What only the night tests, the app opened from outside, a session starts
on its branch through the GitHub API when its change touches
`internal/outside/`, `probe_outside.go`, the Unlock link or
`scripts/bundle-macos.sh`, the way it starts the speech benchmark.

A push to main runs every job but `macos`, since the night runs it. The
build to update to saves the app's cache on main after its upload, which
the nightly app opened from outside starts from.

## What follows

A push that changes no `_darwin` file, `go.mod` or `go.sum` takes one Mac
at most, the build to update to, where it took three. The Updates page
offers a build as soon as before, and on main about 3 to 6 s later,
the cache saved after the upload. A push that changes the Mac's own code
or a module takes two.

What only the night finds: a change in code every system builds that
fails only on a Mac's disk or with the ffmpeg built for the Mac, and a
change that breaks the app opened from outside. It is found the next
morning on main, or before the merge when a session starts this
workflow on its branch.

It was wrong if the night finds a Mac failure more than once a week that
a pull request merged that day caused, or if a push still waits on a Mac
as often as before.
