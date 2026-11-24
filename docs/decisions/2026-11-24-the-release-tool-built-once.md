# The release tool is built once and downloaded

Builds on
[2026-11-14-update-decisions-in-go.md](2026-11-14-update-decisions-in-go.md).

## The problem

The jobs that hold a key or a token that writes run
`cmd/framefairy-release`: the job that signs updates, the two that write
the channel list, and the two that open issues. Each built it with
`go run`, from setup-go's cache. Any run on main can write that cache, a
run that builds a dependency nobody has run before included, and Go
trusts what it finds there without checking it again. So the tool that
signs could have been what such a run left behind.

The first answer turned the cache off in those five jobs. Each then
compiled the tool from nothing, about 12 s, and a publish job grew from
37 to 52 s by 10 to 20 s, on every push. The user, on that pull request:
"Why do we have to build the release tool though? I feel like this
should just be a very small binary that's either already baked into a
container or image, or that's downloaded from an artifact source." And
then: "this is just a step backwards because we're disabling caches ...
build our idea that we just mentioned above into this very pull
request."

What was measured: the tool depends on two packages of ours,
`cmd/framefairy-release` and `updates`, and on `go.mod` and `go.sum`.
Its code changed four times in the month before. A cold build took
12.9, 20.9 and 13.7 s in a cloud session, and 0.5 to 0.7 s warm.

## Five simulations

1. **Who may know about whom.** The jobs that use the tool should not
   know how it is made. One composite action,
   `.github/actions/release-tool`, puts `framefairy-release` on the
   path, and the jobs run it by name. Behind the action, a workflow of
   its own, `release-tool.yml`, builds the tool on main whenever what it
   is built from changes, has GitHub sign it with a build attestation,
   and keeps it as an asset of a release, named by a key of what it was
   built from. The action works the key out of the checkout, downloads
   the asset of that key and checks that the attestation says main's
   `release-tool.yml` made it. With no asset, or one that fails the
   check, it builds the tool from the checkout, with no cache. Names
   that cross: the action's name, the release's name, the key. Next
   changes: a new package the tool imports is one more path in the key,
   which a test holds. Cost: one workflow, one action, five jobs changed.
2. **What changes together.** The tool changes with its code, so it is
   built with its code: in CI on every push to main, as a job of
   `ci.yml`, and the publish jobs take the binary from that run as an
   artifact. But the publish jobs run from other runs of other
   workflows, by `workflow_run`, so each would need to find the right
   CI run of the right commit, which may still run or be cancelled.
   That is a search where the key in the first is one lookup. And
   `ci.yml` holds no write token, which an attestation needs.
3. **Where state lives.** The CI image, `.github/ci-image/`, is kept in
   the registry by a hash of its recipe. The tool could be built into
   it. But the image changes when its Dockerfile changes, and the tool
   four times a month: tying them rebuilds an image of hundreds of MB for
   a 10 MB tool, and the publish jobs run on a plain runner, not in the
   image, so they would pull it, which takes longer than the build it
   saves. The state that fits the tool is a file named by what it was
   built from, the key of the first.
4. **What runs together.** The jobs of one run of `publish.yml` build
   the tool once in a first job and hand it to the others. That saves
   one build of two in a completed run, nothing in `announce`, which
   runs alone, and nothing in `security.yml` and `tell.yml`. It keeps
   the build on every push.
5. **What a test has to build.** The trust is the attestation, so a test
   can only hold the shape: that the key covers every package the tool
   imports, that the workflow that builds it starts when any of them
   changes, and that no job that holds a key builds the tool from a
   cache. The attestation is checked by `gh attestation verify` on every
   use, against the workflow and the branch, so a binary a branch put
   there is refused. This is the first again, with its tests.

## What converges

A binary built once, named by what it is built from, and checked on
every use, the first and the fifth. The second and the fourth build it
as often as before or need a search across runs. The third ties it to
the wrong thing. From the fourth, the fallback stays: a job that finds
no binary builds the tool itself, with no cache, so a change to the tool
publishes its own build too, before `release-tool.yml` has finished.

## The decision

    the five jobs:      framefairy-release <command>
                                  |
    .github/actions/release-tool: key from the checkout, download,
                                  gh attestation verify, or build
                                  |
    release-tool.yml on main:     build, attest, upload as
                                  framefairy-release-<key>
                                  |
    cmd/framefairy-release, updates, go.mod, go.sum

The key is the first 16 hex digits of the SHA-256 of the git objects of
those four paths, which git already has, so no Go is needed to work it
out. The release is `release-tool`, a prerelease that is never the
latest, and it keeps only the newest asset: a job of an older commit
finds none and builds.

The check is `gh attestation verify` with the repository, the signer
workflow `release-tool.yml` and the source ref `refs/heads/main`. Any
branch can upload to the release, because a push runs the branch's own
workflow file with whatever it asks for. It cannot make an attestation
that says main.

## What follows

- A job that holds a key runs a tool that main's own workflow built
  from main's own code, with no cache anybody else wrote, and says so
  in its log.
- It downloads about 10 MB and checks a signature where it set up Go
  and built: a few seconds where it took 10 to 20, and before this,
  with the cache, 5 to 17.
- When the tool changes, the first jobs after the merge build it
  themselves, as before, until `release-tool.yml` is done.
- `internal/workflows` fails when the tool imports a package of ours
  the key does not cover, when `release-tool.yml` does not start on a
  change to one of them, and when a job that holds a key builds the tool
  any other way.
- Wrong if: the check refuses the binary on main for a reason of its
  own, which the log shows as a build every time. Then the jobs are as
  slow as the first answer, and as safe.
