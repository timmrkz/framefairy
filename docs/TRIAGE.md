# Triage

How the open issues are looked over, and how a problem that keeps
coming back is kept as one issue until its cause is gone. Read it before
an issue is logged or picked to work on. The labels it gives are in
Priority and size in [WORKFLOW.md](WORKFLOW.md#priority-and-size).

## Root causes only

Every fix takes the cause away. A change that leaves the cause in place
and only ends what it did this time, a place that gives up on it, a
longer wait, a case skipped, is a workaround, and none is made here.
A workaround closes its issue and leaves the cause to come back under
the next one, and nobody sees that it is the same thing again.

When the cause lies outside our code, in a driver, a runner or a tool,
the cause in our code is what lets it reach the person or the test. That
is fixed once, in one place, for every path it can come through. When
the cause is not known yet, finding it is the work: the trace first,
then the order the code assumes, see section 7 of the
[testing skill](../.claude/skills/testing/SKILL.md). A pull request that
only helps find it, a log line or a trace, says so and closes nothing.

## Every week

A session looks over the issues once a week, started by a routine on
claude.ai. It changes no file, so it opens no pull request. It works in
this order, because what happens now comes first:

1. **Main's CI runs of the last seven days.** Every red run is matched
   to an issue. A red run with none gets one.
2. **Labels.** Every open issue without a priority or a size gets one
   from the table in [WORKFLOW.md](WORKFLOW.md#priority-and-size). An
   issue whose facts changed is looked at again, a flaky test seen
   failing again for example.
3. **What can close.** An issue that main has done is closed as done.
   One whose reason is gone, because a later decision went against it
   or what it names no longer exists, is closed as not planned. A
   duplicate is closed as one, and the issue that stays gets a comment
   with what the other added. A duplicate is the same failure as an
   issue that is still open. The same failure after its issue closed,
   or the same cause in another place, is an incident under a problem,
   see below. An issue a session opened that is mostly done is
   rewritten to what is left. One Tim opened gets a comment that says
   what is left. Each closing says in one line which pull request or
   decision closed it. A closed issue with a sub-issue still open is
   reopened, a problem or any other parent, because a parent closes only
   after the last of its sub-issues.
4. **Problems that come back**, in even weeks of the year, see below.

Tim's labels and Tim's words are never changed. The triage ends with a
reply to Tim that lists every change, each linked, and he checks them.

## Problems that come back

When the same thing goes wrong again and again, each time under a new
issue, every fix looks done and none of them is. Seen together, the
issues say what is broken underneath. On GitHub's virtual Mac the media
engine hung in the decoder probe, #282, then in the video preview's
decoder, #553, in the render, #570, in a test's reference frame, #581,
and in the decoder again, #638. Each pull request made one place give up
on it, which was a workaround, and the next hang came in a place that
did not.

This is how
[ITIL's problem management](https://en.wikipedia.org/wiki/Problem_management)
keeps them apart. One time something goes wrong is an **incident**: a
failed run, a bug Tim saw, a test that hung. It is an ordinary issue.
The cause behind incidents is a **problem**. ITIL opens a problem from
one incident too. Here it takes two, because what matters is what comes
back. A problem is an issue of its own, labelled `problem` beside its
kind, its priority and its size, and its incidents are its
[sub-issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues).
ITIL also lets a workaround close an incident while the problem waits.
Here it does not, see Root causes only above.

- **Before an issue is logged,** the closed issues are searched as well
  as the open ones, by the failing test's name, the function the stack
  stopped in and the words of the error. When the new incident has the
  cause of an earlier one, it goes under that cause's problem. When
  there is no problem yet, one is opened, and both incidents go under
  it. The same cause means the same thing breaks underneath, even in
  another test, another path or another part of the app.
- **A problem says** the cause as far as it is known, every fix tried
  so far and why the cause stayed, and what would take the cause away in
  one place. Its priority is the highest of its incidents.
- **An incident under an open problem is taken up as its problem.** The
  pull request takes the cause away and says `Fixes #N` for each
  incident it ends.
- **A problem closes last,** once the last of its sub-issues is closed
  and not before. Thinking the cause is gone is not enough while one
  incident under it is still open. The pull request that ends that last
  incident says `Fixes #N` for the problem too. A problem closed while a
  sub-issue of it is open is reopened, so the whole tree is worked
  through.
- **When the cause comes back later,** the new incident goes under the
  problem, the problem is reopened, and the work starts over from the
  top. A reopened problem means the cause named was not the whole cause,
  and finding the rest comes first.
- **In even weeks the triage** reads every issue opened or closed in the
  last fourteen days, puts each that shares a cause with another under
  its problem, and opens the problems that are missing. Its reply to Tim
  names the problems that had a new incident, the ones reopened, and
  the problems closed in those fourteen days that have had no incident
  since.
