# One Mac app build per push

Issue #963. Builds on
[2026-11-08-one-make-target-per-ci-job.md](2026-11-08-one-make-target-per-ci-job.md),
which it changes in one place: the checks from outside the app no longer
run in a job of `ci.yml`.

## The problem

Tim, 20 November: "sometimes it appears that we have a lot of resource
congestion", and then: "consolidate the Mac-related workflows so that we
can run CI faster with fewer resources constraining other builds."

The account runs five Mac jobs at once, for every branch together. Every
push to a pull request and to main took three:

- `macos` in `ci.yml`, `make unit` on a Mac.
- `outside` in `ci.yml`, `make outside WERROR=1`, which builds the app
  with `make all` and then the app with the probe.
- `build` in `builds.yml`, `make app WERROR=1`, which builds the app with
  `make all` and bundles it, for the Updates page.

So two Macs built the same app of the same commit, with the same flags
and the same cache. Read from the jobs of #962's last commit, 142b06a,
one run each:

| Job | setup, s | make, s | whole job, s |
| --- | ---: | ---: | ---: |
| `outside` | 53 | 73, of it 45 for `make all` | 134 |
| `build` | 51 | 49 | 116 |

From the runs of CI on main, 14 to 20 November: a Mac job of an
outdated run waited 2491 s in the queue in all, `outside` of run 3330
never got a Mac in 34 minutes, and `outside` of run 3338 waited 636 s.
CLAUDE.md already says a push takes two Mac jobs, the build with the app
opened from outside and the Go tests. The build of `builds.yml` was
never counted.

## Five simulations

1. **Who may know about whom.** `ci.yml` knows the checks, `builds.yml`
   knows the build to update to, and neither knows the other. Each Mac
   job stays and builds what it needs. Three Mac jobs a push. Nothing
   moves, nothing crosses. It costs the Mac queue, which is the problem.
2. **What changes together.** The app the Updates page offers and the
   app the checks from outside open are one build, so one job builds it:
   the build of `builds.yml` runs `make app`, packs the zip, and then
   `make outside`, whose `make all` finds every package compiled and
   only links the programs again, without the channel's version, since
   only a build of one's own takes the local dispenser's test keys. `ci.yml` loses `outside`. What crosses is nothing
   new: the job already has the checkout, the cache and the tools
   `outside` needs. Two Mac jobs a push. A change to the checks from
   outside touches `builds.yml` and `internal/outside`, as a change to
   the build does today.
3. **Where state lives.** The zip is the state, so `outside` downloads
   the artifact `build` leaves and opens that. But `make outside` opens
   an app with the probe built in, which the zip a customer gets must
   never have, so it builds a second app anyway, and still on its own
   Mac. Three Mac jobs, and `ci.yml` waits on a run of another workflow,
   which a pull request in conflict never starts.
4. **What runs together.** One Mac job runs everything a Mac is for:
   `make unit`, `make app` and `make outside`. One Mac job a push. It
   takes about 230 s of tests, 50 of `make all` and 30 of `outside`,
   about 6 minutes with its setup, over the five a job may take, and a
   test that fails would hold back the build to update to, which today
   no Go test does.
5. **What a test has to build.** `make outside` needs the app built,
   bundled and registered with Launch Services, and the dispenser. The
   build of `builds.yml` has all of it but the dispenser, which `make
   outside` builds itself. So the check is cheapest where the app is
   built already, the same answer as 2.

## What converges

2 and 5 are the same design. 3 and 4 fail on what a job may take, how
long or what it may hold back. 1 is what was there. Kept from 1: `ci.yml`
keeps every check that is a test of the code, `macos` among them, so
the Go tests on a Mac still never hold back a build.

## The decision

```
builds.yml build   one Mac a push, of a pull request's channel or main's
  make app         the app to update to, packed and uploaded first
  make outside     the same app with the probe, opened by macOS
ci.yml macos       the Go tests on a Mac, the other Mac a push
```

The zip is uploaded before the checks from outside run, so they never
change what it holds. The run of `builds.yml` ends after them, and
`publish.yml` signs only a run that ended well, so a build whose checks
from outside fail is not offered on the Updates page until they pass.
That was put to Tim on 20 November.

The checks from outside run where the build to update to runs: on every
push of a branch with an open pull request that changes the app against
its channel's build, and on every push to main. A pull request whose
first push came before it was opened gets them from its next push, or
from main once it is merged, as its build does. `ci.yml` leaves the
cache of `go-v2-app` to the build, which saves it on main once the
checks from outside passed, and on any branch when it found none.

## What follows

Every push takes two Mac jobs where it took three, and one setup of
about 50 s and one `make all` of about 45 s less. A build to update to
ends about 33 s later, and the Updates page offers it that much later.
Measured on the first run that passed, one run: `make outside` took 32 s
after the upload, relinking the programs included, and saving the cache
1 s.

It costs three things the checks from outside had in `ci.yml`. The
first commit of a pull request pushed before the pull request was opened
gets none until its next push, though in 13 recent pull requests every
head commit was built. They test the branch as it is, where `ci.yml`
tested the pull request merged with main, so what main changed meanwhile
is checked on main after the merge. And a pull request from a fork gets
none, since `builds.yml` runs only on a push to this repository.

It was wrong if a push still waits on a Mac as often as before, if the
build to update to is held back by a failing check from outside more
than once a week, or if a change to the app reaches main without the
checks from outside having run on it, and then fails them there.
