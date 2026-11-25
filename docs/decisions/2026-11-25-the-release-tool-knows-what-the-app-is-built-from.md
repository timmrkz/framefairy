# The release tool knows what the app is built from

Builds on
[2026-11-20-a-mac-only-for-mac-code.md](2026-11-20-a-mac-only-for-mac-code.md)
and [2026-11-24-the-release-tool-built-once.md](2026-11-24-the-release-tool-built-once.md).

## The problem

A pull request that changed only the release tool, which runs on Linux
in CI and never on a Mac, started the four walks and the Mac build to
update to. The Go tests on a Mac already start only for code only a Mac
builds. What is left is that any Go file counts as the app: the filter
`go` of the job `changes` starts the walks, and `needsBuild` counts every
file but the docs as a change to the app.

A first answer listed the folders the app is never built from in a
filter of `ci.yml`, held by a test to what the app imports. The user
rejected it: "this can break with every single commit", and CI should
not rely "on those easily modified code paths. In the best of all cases
CI doesn't even rely on any code organisational details, like file
paths." Then, for this change: not "any Mac job for things like
building the release tool", and the release tool "available as an
artifact with attestations ... download it for all of the jobs".

What was measured, on main at ca94322:

- Of the last 150 pull requests merged, 31 October to 25 November, one
  changed only Go the app is not built from. That share grows with the
  licence server and the packaging still to come, which never run on a
  Mac.
- The walks run `go test -run TestWalks` of the app's own package, and
  start `framefairy-frames` beside it. So they are built from what those
  two import, and from the app's own tests.
- `go list -deps` of the two programs names 15 packages of ours for the
  Mac, and 19 with the app's tests. The other 18 packages of the module
  are tools, the licence server, the training and the tests' own checks.
- The job `changes` takes 5 to 10 s, and `image` waits for it, though it
  reads none of its outputs.

## Five simulations

1. **Who may know about whom.** CI should know kinds of files, never
   folders. The one thing that knows what the app is built from is Go,
   from the imports. So the release tool, which already decides whether
   a push needs a build, reads the imports of the commit with Go's own
   parser and judges each changed file. The programs the app ships are
   already named in the tool, in `linked`, which `check` holds to the
   real build, and Go names a program after the last element of its
   package's path, so the tool finds them by name wherever they live.
   Names that cross: the command `walks`, its output `walks`. Next
   changes: a new program in the app is a new entry in `linked`, which
   the check of every build demands anyway. Cost: one file in the tool,
   one step in `changes`.
2. **What changes together.** The list of folders changes whenever a
   package is added or moved, which is the trouble. What changes with
   the app is its imports. `go list -e -deps` of the two programs is the
   toolchain's own answer, and it needs no network: with no module cache
   and `GOPROXY=off` it names the same 15 packages in 0.09 s. Its cost is
   a Go 1.27 toolchain set up in `changes`, 10 to 20 s, in a job that has
   none, and a `go` command to run where the channel list asks about
   other commits. The tool's parser comes to a superset, every file
   whatever its build tags, which is the safe side, with nothing to set
   up. A test holds it to `go list` for the Mac and for Linux. This
   converges on the first.
3. **What the build runs, not only what it links.** The app is built
   with more than it imports: `make` runs `notices/gen`, which writes the
   licences the app carries in `dist/`. A change to it changes the app,
   and no import shows it. Reading the Makefile for what it runs would
   be reading shell, and it runs the release tool too, for `make
   update-key`. Go has its own word for a program a package's build
   runs: a `//go:generate` line. So `make` runs `go generate` for the
   app, the line beside the app's `go:embed` names `notices/gen`, and
   the tool follows `go run` in every such line of the app's packages
   as it follows an import. A line it cannot judge, anything but `go
   run` of a package of ours, is a doubt, and everything runs. The
   answer is worked out from the commit every time, out of `git archive`
   of its Go files, 6 MB in 50 ms, so nothing is kept that could go
   stale, and the channel list, which asks about other commits from
   main's checkout, reads each commit's own Go the same way.
4. **What runs together.** The decision could be made inside each walks
   job, which has Go already, and end it early. That saves no runner,
   since four jobs still start, and it decides the same thing four
   times. In `changes` it is decided once, before anything starts. So
   `changes` gains the checkout and the signed tool, a few seconds, and
   `image` stops waiting for it, so nothing waits longer than before.
   The tool runs only when the change is Go alone: an interface change
   starts the walks anyway.
5. **What a test has to build.** The answer is only safe if it never
   misses a package the app is built from. So: a table of every kind of
   file, a moved program, a program missing, a file moved out of the
   app, which git lists under both names only with `--no-renames`, a
   `go generate` line, Go that cannot be read, each answering on the
   side of running, and the test against `go list` on this repository's
   own code. A file that is not Go always reaches the app, a script, the
   Makefile, a file a package embeds, since nothing can say what reads
   it. That is the first with its tests.

## What converges

The release tool answers from the imports and the `go generate` lines,
worked out of the commit on every run, with no folder named anywhere:
the first, the second, the third and the fifth. The second is the same
answer from the toolchain, for the cost of setting it up. The fourth
places it in `changes`, once, with `image` no longer waiting.

## The decision

    ci.yml changes:      dorny/paths-filter for the kinds of files,
                         framefairy-release walks for Go alone
    builds.yml which:    framefairy-release build-plan, needsBuild
    publish.yml:         framefairy-release channels, needsBuild
                                  |
    .github/actions/release-tool: main's signed build, or one built here
                                  |
    reach.go:            the programs in linked, found by name,
                         every package of ours they import or their
                         go generate runs, read out of the commit
                         with git archive

A change reaches the app when it is not the docs and is either a file
that is not Go, or a Go file of a package the programs import or their
build runs, or a test of a program itself for the walks. Every doubt
runs: a commit that cannot be read, Go that cannot be parsed, a `go
generate` line that is not `go run` of ours, no program found. The
build never ships a test, so a pull request that only changes tests, or
the release tool, or the licence server, takes no Mac for a build. A
pull request with no build yet is judged against main where it
branched, so not even its first push takes one. The
`builds.yml` job `which` takes the signed tool too, instead of building
it with `go run`.

## What follows

- A pull request of the release tool, the licence server, the training
  tool or tests alone starts no walk and no Mac build. The Updates page
  keeps offering the build there is, which is the app as it stands.
- `changes` gains a few seconds on a change of Go alone, and none on
  any other. `image` and the jobs after it no longer wait for it.
- A change to the release tool itself builds the tool in the job, about
  12 s, until main has a signed build of it.
- Nobody keeps a list. A package added, moved or renamed is judged by
  its imports on the next run.
- A program the app's build runs is run through `go generate`, or the
  tool cannot see it. That is the toolchain's own place for it, and the
  one program the build runs today, `notices/gen`, is there.
- Wrong if: a walk or a build that should have run did not. A file that
  is not Go always runs everything, so it would take a program of ours
  that the app's build runs some other way than `go generate`. Or `changes` takes so long that the jobs after
  it start later than `image` lets them.
