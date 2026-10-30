# A video's state lives in one session in the interface

Issue #606, from the architecture review of 29 October 2026 (#603).
Decided on 30 October 2026, in the pull request that makes its first
slice. The slices after it name this record.

## The problem

Tim asked on 29 October for the issues of the architecture review that
matter most for reliability and long-term maintenance, a large one in a
pull request of its own. #606 is one of them: "Go owns a video's state
and sends it as one snapshot", so the workspace stops guessing from pings.

Measured on main at 127fbef:

- `Episode.svelte` is 4,585 lines with 35 effects. Five of them notice
  that work moved on by keeping a value from before in a plain variable
  and comparing it with the next: `foundHeard`, `wasWhole`,
  `wasTranscribing`, `searchRan`, and the renders in `waiting`. Two more
  notice that a search started, `followed` and `searchSeen`.
- The clip list is read again from 11 places, `refreshClips`, and the
  whole video from 7, `load`. Three tickets, `Newest`, keep the answers
  to those reads in order, and `OnTheWay` and `unread` keep the clips
  between the job saying it wrote one and a read that has it.
- Two polls: the job list every 5 s while anything runs, in
  `lib/state.svelte.ts`, and the workspace's own timer every 2 s, which
  reads the video while it is heard and the clips while they are found.
- Go's `episode` event carries only a path. A job's news is a whole `Job`,
  sent from 12 places in `cmd/framefairy-app/jobs.go`, which copies ten
  fields of `engine.JobRecord` and writes them again as the job goes.

An effect that compares the value from before runs on every job event,
about once a second, and sees only the newest state after the batch: a
search that ends and a new one that starts before the effect runs are
one search to it. Each new kind of work has added an effect, a ticket or
a counter of its own, which is how the family of #323, #331, #431, #436,
#444, #450, #513 and #572 came about.

## Five simulations

### 1. Who may know about whom

Go knows the state of a video, the interface shows it. Go assembles one
value for a video, its status, its clips with the ones on their way, how
far it is heard and its jobs, with a revision that grows on every change,
and sends it as the `episode` event, at most four times a second with
the last change always sent. The interface keeps it in one session per
open video and the workspace is a view of it. Crossing the boundary: one
event and one type, in place of the path the event carries now. The
bindings `Episode`, `Clips` and `Coverage` stay for the first read. The
next fact about a video is a field of that type. Cost: about four pull
requests, Go and the interface each in its own.

### 2. What changes together

A job's life is told where its news lands. The job store already applies
every snapshot of a job, in order, and drops an older one. It hands
each snapshot it keeps on, with the one it replaced, and a session per
open video says what moved: a clip written, the hearing over, the search
over, the whole video heard, a render over. No change on the Go side.
One pull request moves the five effects. The polls and the tickets stay,
because the workspace still has to guess what to read again.

### 3. Where state lives

One home for each piece of a video's state in the interface. Today
`status`, `clips` and `coverage` are kept in `Episode.svelte`, each with
its own ticket, and the jobs in the store. All of it moves into the
session, and the components read from it. A session kept in a map in
`lib/` would let a workspace that sleeps, and the sidebar, read the same
one. It moves 3 states and 3 tickets out of `Episode.svelte`, and every
read of them, 136 lines that name `status`, `clips` or `coverage`.

### 4. What runs together

Go sends a revision and what changed, `{episode, rev, changed}`, at most
four times a second, and the interface reads only what changed, as
`docs/DESIGN-REVIEW.md` proposed on 6 October. The bindings stay what
they are. But every change costs a round trip, and two reads in the air
answer in either order, so a ticket stays, kept by revision. The
interface still decides what to read, which is what the issue asks to
end.

### 5. What a test has to build

The session is a plain class that takes the job store, later the
events, from outside, and is tested in Vitest with neither Svelte
components nor Go, the way `lib/state.svelte.ts` is. Go's snapshot is
made by one function from the queue and the work folder, tested with a
memory disk. The walks prove the two together. So the transitions are
worked out in the session from two snapshots, and nothing about them is
left in a component.

## What converges

- One session per open video in `frontend/src/lib/`, the one place a
  video's state lives in the interface: 1, 3 and 5, and 2 as its seam.
- What moved is said once, by the session, as it applies the news,
  never by an effect: 2, 5, and 1, where the session compares two
  snapshots.
- Go sends the whole snapshot, with a revision, coalesced: 1 and 3. Only
  4 pulls. A pull keeps a ticket and a round trip for every change, and
  leaves the interface deciding what to read, so it is dropped.
- A `Job` holds the engine's record instead of copying it, each state
  saying one thing: 1 and 3, and asked for by the issue.
- A map of sessions shared with the sidebar is only in 3. The sidebar
  reads the library, not a video's state, so it is left until something
  needs it.

## The decision

The interface, each layer using only the ones under it:

```
screens/Episode.svelte     a view: reads the session, asks Go through api.ts
lib/session.svelte.ts      one video's state, kept by revision, and what its work did
lib/state.svelte.ts        every video's jobs, each snapshot kept handed on
lib/api.ts                 the calls and the events of Go
```

Under them the app's Go side, which sends a video's state as one
snapshot, and the engine, which keeps each job's record.

In four slices, each a pull request:

1. `EpisodeSession` in `frontend/src/lib/session.svelte.ts`, which hears
   every snapshot the job store keeps through `JobStore.watch` and says
   the moves of the video's work. The five effects that noticed work
   moving on become its five moves, unchanged in what they do. This
   pull request.
2. A `Job` holds its `engine.JobRecord`, and each state says one thing.
   Go only.
3. Go sends a video's state as one snapshot with a revision, coalesced,
   and the session keeps it when its revision is newer. `status`,
   `clips`, `coverage` and the video's jobs move into the session.
4. The tickets, `OnTheWay`, `unread`, the effects that notice a start
   and both polls go. One `startJob` keeps the pending state of every
   control from the video's jobs, as Tim's comment on the issue asks.
   This fixes #513.

## What follows

- It costs four pull requests, the third the largest.
- It buys a workspace that shows what Go says instead of working it out,
  so a new kind of work is a field and a move, not an effect, a ticket
  and a counter.
- Slice 1 already changes one thing: the session hears every snapshot,
  so a search that ends and another that starts in the same batch of
  news are two searches, where the effect saw one.
- It was wrong if the snapshot of a video with many clips is too large
  to send four times a second, which slice 3 measures before it sends
  it, or if a component needs a value from before that the session does
  not keep.
