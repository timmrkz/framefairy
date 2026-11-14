# What decides a build lives in Go, not in shell

Issues #838, #839, #840 and #841, in one pull request, #842.

## The problem

Fixing the four update bugs, #842 first added `scripts/build-ended.sh`,
its test with a stand-in `gh`, and about 60 lines of shell inside
publish.yml. Tim, 14 November: "why are we starting to write a bunch of
shell scripts again in this pull request ... we are now creating a whole
new mess full of shell scripts again in order to address those small
issues ... how can we then prevent agents in the future from writing
those stupid shell scripts all over the place for things that should
actually be simple?" He chose to move every update decision into Go in
this pull request.

What decided a build on main at b0488d0, counted from the files:

| Where | What it decides | Lines of shell |
| --- | --- | ---: |
| `scripts/needs-build.sh` | whether a commit needs a build of its own | 64 |
| `scripts/needs-build-test.sh` | its test, with a repository it makes | 89 |
| `scripts/channel-list.sh` | the channel list: entries, open pull requests, conflicts, newest commits, the two copies | 122 |
| builds.yml, steps `which`, `key`, `needs`, `what` | what a push is a build of, and whether to build | 66 |
| publish.yml, steps `which`, `key`, `zip`, the status steps, `unstick`, the removal of old zips | what a build is, whether to sign it, what to mark | 96 |

Each piece of shell reads GitHub through `gh` and `jq`, so none of it can
be tested without a stand-in for both, and the CI image has no `jq`. A
mistake in it shows only on main, after the merge, because publish.yml
always runs from main. The issues are each one such mistake.

## Five simulations

1. **Who may know about whom.** Every decision moves into the Go program
   that already signs the builds and writes the list,
   `cmd/framefairy-release`. It reaches GitHub through one interface,
   `gitHub` in `github.go`, which the tests replace. A workflow step runs
   one command of it. Names that cross: the commands and their flags.
   Cost: one program grows by about 600 lines of Go and its tests, two
   scripts and their test go. The next change to how a build is chosen
   is a Go change with a Go test.
2. **What changes together.** Keep the shell, and test it with an
   established shell test tool, bats or shellspec. The rules stay where
   they are and gain tests. Cost: a test tool to pin and install in the
   CI image, `jq` too, and stand-ins for `gh` that copy GitHub's answers
   by hand. The shell goes on growing by copying, which is how #842
   began.
3. **Where state lives.** The channel list is worked out on request by a
   small server, the licence server's sibling, from GitHub's state, and
   the app asks it. Nothing writes a list at all. Cost: a server that has
   to be up for every app that checks for updates, a new thing to run and
   pay for, and the app asking a server of ours instead of a plain file,
   which docs/UPDATES.md chose against.
4. **What runs together.** The decisions move into JavaScript inside the
   workflows, with `actions/github-script`, which brings GitHub's client.
   Cost: code inside YAML that no test reaches, in a language the rest of
   the build does not use.
5. **What a test has to build.** A second Go program for CI's own
   decisions, beside `framefairy-release`, which keeps only the key and
   the signing. Cost: two programs that both read the channel entries and
   both talk to GitHub, the second copy of a primitive that the rules
   forbid, and every change to a build touching both.

## What converges

1. Each decision is Go in `cmd/framefairy-release`, tested with `go test`
   against a stand-in GitHub and, for what reads two commits' files, a
   repository the test makes. `github.go` is the one place it talks to
   GitHub's REST API, checked against a test server that answers the way
   GitHub does.
2. A workflow step runs one command and does nothing else of its own.
   `channel-list.sh`, `needs-build.sh` and `needs-build-test.sh` are
   removed. The scripts that stay build the programs we ship or set up a
   machine, and none of them decides what a build is.
3. `internal/workflows` holds both, so the next agent cannot copy the
   old way back: a script in `scripts/` that is not on its list fails,
   and a workflow step of more than five lines of shell fails, with a
   message that points here. The steps already longer than that, outside
   the update workflows, are listed with their length today, and may
   only get shorter.

Two did not converge. 3 moves the problem to a server we would have to
run. 2 keeps the shell and its stand-ins.

## How GitHub is reached

The review asked about one more way, inside design 1: reach GitHub
through `gh api` or the `go-github` library, behind the same
interface, rather than a client of our own over `net/http`.

- `gh api` from Go is a program started for every question, the shell
  again under another name, and a test of it needs a stand-in `gh`.
- `go-github` is a large dependency, a module of its own with a release
  every few weeks, for twelve calls. Each call would still be written
  here, behind the interface, and checked against a test server. What
  it saves is the paging, the upload address and the redirect of a
  download, which `TestGitHubIsAskedWhatTheToolNeeds` holds.

So the client stays our own, in `github.go`, and the test server checks
every call it makes. If it ever grows past these calls, `go-github` is
the step after it.

## What it costs

- One pull request, #842, with the four fixes it already holds.
- The `which` job of builds.yml checks the repository out and sets up Go,
  which the Mac job did before. A commit that needs no build no longer
  starts a Mac at all.
- publish.yml sets up Go in each job that decides, which `list` and the
  signing did already.
