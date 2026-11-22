# A run at night that fails tells of itself

An issue on the runs at night that fail unseen. Builds on
[2026-11-14-update-decisions-in-go.md](2026-11-14-update-decisions-in-go.md)
and
[2026-11-20-a-mac-only-for-mac-code.md](2026-11-20-a-mac-only-for-mac-code.md).

## The problem

Since a Mac runs only for Mac code, a change that breaks the Mac's disk, the ffmpeg built for
the Mac or the app opened from outside fails no pull request. It fails
`mac-nightly.yml` on main the next night, a run nobody waits on. The
same was true of `fuzz-nightly.yml`, `walks-nightly.yml` and the weekly
`security.yml` before. On a failure they keep an artifact and nothing
else. Sessions hear only the events of their own pull requests, and the
weekly look at the issues in [TRIAGE.md](../TRIAGE.md) reads main's
runs once in seven days. So "found the next morning", which the record
of that change promised, held only if someone looked.

The user, 21 November, on a card in the thread of that change: "Yes, one issue, in a
new PR", a failed nightly opens or updates one issue by itself.

## Five simulations

1. **Who may know about whom.** The workflow that failed knows it
   failed, so it tells: a last job, `tell`, needs every other job, runs
   when one failed on main, and calls `tell.yml`, one workflow that
   every workflow on a schedule calls, the way they call
   `ci-image.yml`. It runs a command of `cmd/framefairy-release`, the
   program the workflows already run to ask GitHub, through its
   `gitHub` interface, its REST client and the fake its tests use. The
   command opens one issue for the workflow, or adds the run to the open
   one as a comment. A guard in `internal/workflows` holds every
   workflow on a schedule to it. Written out in each workflow instead,
   the same 24 lines four times, the pins, the time limit and the step
   would drift apart, which the first round of this pull request did
   and the review caught.
2. **What changes together.** Telling of a failure is not about a
   build, so a program of its own, `cmd/framefairy-ci`, holds it. But it
   needs what `framefairy-release` has already: the token, the REST
   call, the errors, a fake for the tests. A second client is a copy
   that drifts, which CLAUDE.md's one primitive per thing forbids, and
   moving the client into a package both share is a refactoring bigger
   than the change.
3. **Where state lives.** What failed is in GitHub's record of runs
   already, so nothing is added: GitHub mails whoever last changed the
   schedule of a workflow when its run fails. It costs nothing, but it
   reaches one inbox, the user's, since sessions push as them, and they do not
   work from mail. No session reads it, and nothing of it is an issue,
   which is where the order of work in CLAUDE.md starts.
4. **What runs together.** One watcher workflow, on `workflow_run` of
   the workflows at night, tells of any that failed. One job in one
   file, but its list names the workflows it watches by their names,
   which a new workflow at night has to join by hand, and every run of
   them starts a run of it, failed or not.
5. **What a test has to build.** No code: an action from the
   marketplace that opens an issue from a template, or updates the open
   one. It writes the issue's body again each night, so the nights
   before are lost, it holds a token that writes issues while being code
   we did not write, and what it decides is tested by no `go test`.

`security.yml` kept an issue of its own already, for the tools the app
ships that are behind their newest release, in 17 lines of shell around
`gh`, from before the rule that a workflow decides nothing in shell.
That is the same thing, one issue a workflow keeps, done a second way.
So it moves into the same program as a command, `behind`, beside
`failed`, in this pull request.

## What converges

1 and 4 both tell from GitHub's own word that a run failed, with one
issue per workflow. 1 keeps the telling beside what it tells of, needs
no list of names, and its `needs` reaches every job, which the guard
holds. 2 is 1 with a copied client. 3 reaches no session. 5 loses the
history of the nights.

## The decision

```
every workflow on a schedule   a last job tell: needs every other job,
                               if: failure() && github.ref == 'refs/heads/main',
                               issues: write, actions: read
tell.yml                       called by tell, runs framefairy-release failed
framefairy-release failed      the open issue "<workflow> failed on main" a
                               workflow opened: a comment with the run, the
                               commit and the failed jobs. None open: a new
                               issue, tests, priority/critical, size/medium
internal/workflows             TestAScheduledRunThatFailsTellsOfItself
framefairy-release behind      the weekly issue of tools behind upstream,
                               from security.yml, in place of its shell
```

A run stopped by a newer one is no failure, `failure()` is false for it.
A run by hand on a branch is its session's to read, and tells nobody. The
command never closes an issue: a run that passes the next night says
nothing about why one failed, see A test that fails now and then in
CLAUDE.md. The fix closes it. Only issues a workflow opened are looked
through, so an issue a person gave the same title is never written into.

## What follows

A failure at night is an open issue the next morning, labelled the way a
failing test is, so a session picks it before older work. A failure
that comes again lands in the same issue, one line a night. A run that
fails costs one Linux job of about a minute more, to compile the command.
A run that passes costs nothing, `tell` is skipped.

A run that fails before any job starts, a startup failure, runs no
`tell` either. The likely cause is a called workflow asking for more
than its caller grants, which the guard checks for `tell.yml`. Any other
such run is found by the weekly look at main's runs.

It was wrong if a failure on main reaches no issue, or if issues opened
this way are closed without a fix more than once a month because what
failed was not ours to fix.
