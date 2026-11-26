# The release tool keys itself

Builds on
[2026-11-24-the-release-tool-built-once.md](2026-11-24-the-release-tool-built-once.md)
and
[2026-11-25-the-release-tool-knows-what-the-app-is-built-from.md](2026-11-25-the-release-tool-knows-what-the-app-is-built-from.md).

In 30 words: the tool's key is a fingerprint of its source code, no
password. Anyone may compute it, and it opens nothing. It only says
which code a build is. Trust comes from main's signature alone.

## The problem

The jobs that hold a key download main's signed build of
`framefairy-release`. They found it by a key of what it is built from,
which the action worked out in shell from a list of paths, `go.mod`,
`go.sum`, `cmd/framefairy-release` and `updates`, and `release-tool.yml`
started on a push of the same list. A test held the list to the tool's
imports. That is the kind of list the user rejected for CI: "this can
break with every single commit", and CI should not rely "on those easily
modified code paths". And "none of this should be done in shell
scripts".

What was measured, in a cloud session:

- The tool's inputs changed in 18 of the 203 commits on main in the month
  to 25 November, Dependabot's `go.mod` and `go.sum` included.
- A cold build of the tool took 12.0, 11.6 and 13.4 s. Built by itself,
  `go run -trimpath` and then `go build -trimpath` with the key stamped
  in, it took 12.4, 12.8 and 12.4 s, since the second build finds every
  package in the first one's cache. Without `-trimpath` on `go run` the
  two share nothing and it took 24 to 25 s.
- Asking the signed build whether it fits the checkout, which reads the
  commit's Go with `git archive` and parses its imports, took 111 to
  140 ms over five runs.

## Five simulations

1. **Who may know about whom.** Only Go knows what the tool is built
   from, by its imports, and the tool already reads them for the app in
   `reach.go`. So the tool keys itself: `framefairy-release self` builds
   the tool with the key of its own imports stamped in, and
   `framefairy-release fits` says whether a checkout is still what the
   build was made from. The tool finds its own folder by the import path
   Go stamps into every program. The action downloads one asset of a
   fixed name, checks its attestation, and asks it. Names that cross:
   the asset's name and the two commands. Next changes: an import added
   to the tool changes the key by itself. Cost: one file in the tool,
   one action and one workflow rewritten.
2. **What changes together.** The key could come from the toolchain,
   `go list -deps` of the tool, in the action. But then every job sets
   up Go before it can download anything, 10 to 20 s, which is what
   downloading the tool was meant to save. The tool's own parser comes
   to a superset of the same answer with nothing to set up, which a test
   holds to `go list`. This converges on the first.
3. **Where state lives.** The asset could be named by main's commit, or
   carry the commit Go stamps in with `-buildvcs`, and the tool would
   compare that commit's code with the checkout's. But a job checks out
   one commit and no history, so the older commit's code is not there to
   compare, and asking GitHub for it is a token and a round trip in
   every job. The key in the binary is the answer itself, kept inside
   the bytes the attestation signs. This converges on the first.
4. **Who may vouch for whom.** The checkout, not the downloaded bytes,
   should decide what may run. Before, it did: it named the build it
   wanted, and the attestation had to sign that very name. With no list,
   the checkout cannot name it, so the downloaded build says whether it
   fits, and any build main ever signed could be uploaded again and
   answer for itself. Three ways to narrow that were weighed. Deleting
   the attestation of every older build when a new one is kept: but an
   attestation is a public bundle, kept in Sigstore's public log for
   good, and GitHub stores a bundle for any token that may write
   attestations. Its documentation does not say it refuses one it held
   before, so whoever could put an old build back can likely put its
   signature back too. Deleting adds a write right and a step that can
   fail, and likely stops nobody. Naming main's
   newest build from GitHub's record of `release-tool.yml` runs, which
   no branch writes: a token that reads Actions and a call in every job,
   for the case of a `fits` main once got wrong. Or let `fits` be the
   guard alone: every build main signed is main's reviewed code, and it
   says yes only to the code it was built from. The user asked for the
   most secure way, and the simplest that holds is the last: no
   deletion, no extra right, and every answer but yes ends in a build of
   the checkout. Should a signed `fits` ever be found wrong, there is
   still a way out with no new right: renaming `release-tool.yml`, and
   the signer the action checks, retires every build signed before at
   once, since the signature names the file. The same principle splits
   the work: the job that asks the downloaded build holds a token that
   only reads, and the job that signs as main runs nothing it
   downloaded, only a build of its own checkout.
5. **What a test has to build.** The key is safe only if it never
   leaves out a package the tool is built from, and a download is safe
   only if it is checked before it runs. So: a table of the tool's
   inputs, its imports, those in turn, a moved tool, what its `go
   generate` runs, and Go that cannot be read, which gives no key and so
   a build. The key of this repository's own tool against `go list`. A
   build with no stamp, another key or no key for the checkout does not
   fit. And the workflows: `release-tool.yml` runs only when `builds.yml`
   calls it on main, the action
   reads no paths itself, builds only through `self`, and runs the
   download only once `gh attestation verify` passed, and the job that
   signs runs nothing it downloaded. That is the first with its tests.

## What converges

The tool keys itself from its own imports, stamps the key into its
build, and says whether it fits the checkout: the first, the second, the
third and the fifth. The fourth splits the workflow into a job that asks
and a job that signs, and leaves `fits` as the guard. The job that asks
need not be a new one: `builds.yml`'s job `which` already runs the
action on every push, with a token that only reads. So on a push to
main where its answer was a build, `builds.yml` calls `release-tool.yml`
for the job that signs, and on every other push nothing more runs. A
called workflow signs as its own file, so the signature still names
`release-tool.yml`. No pull request runs it.

## The decision

    builds.yml:         which, on every push, a token that reads: the
                        action. tool, on main, only when which built
                        the tool: calls release-tool.yml
                                  |
    release-tool.yml:   build, signing as main: the action with
                        download false, attest, framefairy-release
                        keep: the asset framefairy-release of the
                        release release-tool
                                  |
    .github/actions/release-tool: gh release download, gh attestation
                        verify, framefairy-release fits, or
                        go run -trimpath ./cmd/framefairy-release self
                                  |
    self.go:            the tool's inputs, go.mod, go.sum and the
                        folders of the packages it imports or its
                        go generate runs, hashed over their git
                        objects, stamped with -ldflags -X, with no C.
                        No key, no build: self fails

Any branch that may write to the release can upload a file to it, but
none can sign as main, so `gh attestation verify` refuses anything main
did not build. A build main did sign, an older one put back, runs in a
job that cannot sign as main, and its own `fits` says no unless the
checkout is the code it was built from. What remains trusted is that
`fits`, main's own reviewed code. If one ever answered wrong, a later
fix would not take that build's signature away: that is the risk left,
and it is why `fits` is small and held by tests. The builds from before
this change still hold their signatures and have no `fits`, so the step
that asks one fails and the action builds the tool.

The key leaves out the system the tool runs on, which the earlier key
held. A build for another system does not start, so the step that asks
it fails, and the action builds the tool. Every job that uses it runs on
Linux today.

## What follows

- No workflow and no action names what the tool is built from. A package
  the tool starts to import is in its key on the next run.
- No job is added to a push. `release-tool.yml` runs only on the pushes
  to main that change the tool's code, 18 of 203 in the last month,
  called from `builds.yml`. No pull request waits on it.
- The jobs that use the tool take as long as before: a download and a
  check, and 12 s to build when the tool changed.
- The asset is called `framefairy-release` from now on. The first push to
  main after this builds it, and `keep` removes the older assets named
  by key.
- No job deletes an attestation, so no step can fail on one that stays.
- A signed build found wrong is retired by renaming `release-tool.yml`
  and the signer in the action, in one pull request.
- A change that makes the tool's key impossible to work out, like a
  `go generate` line in its packages that is no `go run` of ours, fails
  every job that builds the tool on that pull request, instead of leaving
  main a build that never fits.
- Wrong if: a job runs a tool built from other code than its checkout,
  which takes a package the tool is built from that its imports and its
  `go generate` do not name. Or every job builds the tool, which shows
  as `fits: false` in the step that asks.
