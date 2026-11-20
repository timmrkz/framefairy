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
2. **What changes together.** The Mac's code and its tests change
   together, in the same `_darwin` files, so the filter that sees one sees
   both. Code that asks `runtime.GOOS` in an ordinary file hides a Mac
   branch where no filter sees it, so it moves into a `_darwin` file and
   its counterpart. The same design as 1.
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

1 and 2 are the same design, and 3 reaches the same filter with a program
the check makes unneeded. 4 is kept for what no code says is the Mac's:
the full suite on a Mac and the app opened from outside run every night.
5 is the refactor 1 makes unnecessary.

## The decision

```
a pull request    the Go tests on a Mac only when a *_darwin*.go file changed,
                  ci.yml macos, filter mac of the job changes
                  the build to update to, builds.yml build, as before
internal/maccode  a file only a Mac builds is named *_darwin.go or
                  *_darwin_test.go, and no other file asks runtime.GOOS a
                  question a Mac and Linux answer differently
every night       mac-nightly.yml on main: make unit and make outside on a
                  Mac, two jobs, and Run workflow on any branch
```

The check is two tests in `internal/maccode`, which run in `make unit` on
Linux like every other test, so a pull request that breaks the rule fails
on Linux, with the file and line and what to do. A question a Mac and
Linux answer alike, `runtime.GOOS == "windows"`, is no Mac code, and nor
is `runtime.GOOS` handed to a function as a value, like
`SystemMedia(runtime.GOOS)`: the function takes the system as a
parameter, and a test on Linux hands it `"darwin"`.

A push to main runs every job but `macos`, since the night runs it. The
build to update to saves the app's cache on main after its upload, which
the nightly app opened from outside starts from.

## What follows

A push that changes no `_darwin` file takes one Mac at most, the build to
update to, where it took three. The Updates page offers a build as soon
as before. A push that changes the Mac's own code takes two.

What only the night finds: a change in code every system builds that
fails only on a Mac's disk or with the ffmpeg built for the Mac, and a
change that breaks the app opened from outside. It is found the next
morning on main, or before the merge with Run workflow on the branch.

It was wrong if the night finds a Mac failure more than once a week that
a pull request merged that day caused, or if a push still waits on a Mac
as often as before.
