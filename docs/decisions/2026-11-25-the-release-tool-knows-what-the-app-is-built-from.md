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
   the app is its imports. Asking `go list -deps` on every run would be
   the toolchain's own answer, but it needs the modules downloaded, or a
   cache, in a job that has neither, and it would come to the same
   packages. The tool's parser comes to a superset, every file whatever
   its build tags, which is the safe side. A test holds it to `go list`
   for the Mac and for Linux, with no network. This converges on the
   first.
3. **Where state lives.** No state: the answer is worked out from the
   commit every time, out of `git archive` of its Go files, 6 MB in
   50 ms. Nothing is kept between runs that could go stale, and the
   channel list, which asks the same question about other commits from
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
   file, a moved program, a program missing, Go that cannot be read,
   each answering on the side of running, and the test against
   `go list` on this repository's own code. A file that is not Go always
   reaches the app, a script, the Makefile, a file a package embeds,
   since nothing can say what reads it. That is the first with its
   tests.

## What converges

The release tool answers from the imports, worked out of the commit on
every run, with no folder named anywhere: the first, the second, the
third and the fifth. The fourth places it in `changes`, once, with
`image` no longer waiting.

## The decision

    ci.yml changes:      dorny/paths-filter for the kinds of files,
                         framefairy-release walks for Go alone
    builds.yml which:    framefairy-release build-plan, needsBuild
    publish.yml:         framefairy-release channels, needsBuild
                                  |
    .github/actions/release-tool: main's signed build, or one built here
                                  |
    reach.go:            the programs in linked, found by name,
                         every package of ours they import,
                         read out of the commit with git archive

A change reaches the app when it is not the docs and is either a file
that is not Go, or a Go file of a package the programs import, or a
test of a program itself for the walks. Every doubt runs: a commit that
cannot be read, Go that cannot be parsed, no program found. The build
never ships a test, so a pull request that only changes tests, or the
release tool, or the licence server, takes no Mac for a build. The
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
- Wrong if: a walk or a build that should have run did not, which only a
  file that is not Go but is read by name could cause, and such a file
  always runs everything. Or `changes` takes so long that the jobs after
  it start later than `image` lets them.
