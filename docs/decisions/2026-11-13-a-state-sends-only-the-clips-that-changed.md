# A state sends only the clips that changed

Issue #809, in pull request #812.

## The problem

Tim, 12 November: "I want the engine, the front end, and the transport
between those two to be as efficient, fast, and reliable as possible."
The front end review of that night, finding 4, found every state of a
video crossing to the window whole, about four times a second while
anything runs, though its clips seldom change.

Measured on the code as it is, with clips shaped the way the engine
makes them, in a cloud session, three runs each, the same each time:

| clips | state sent whole, KB | state with no clip changed, KB | change |
| ---: | ---: | ---: | --- |
| 10 | 13.0 | 2.9 | 78 % smaller, ± 0 % |
| 40 | 45.6 | 4.7 | 90 % smaller, ± 0 % |
| 120 | 133.2 | 9.7 | 93 % smaller, ± 0 % |

Wails v3 beta 23 puts any event over 8 KB aside and tells the window to
fetch it over HTTP, `maxInlineEventPayload` in its
`event_payload_store.go`, because WebKit on macOS keeps the memory of a
large event spliced into the page. So nearly every state was a request
back to Go and then a parse, 0.11 ms of parse for 40 clips in Node, and
from the first such event on, Wails queues every later event of the
window behind those fetches.

## Five simulations

1. **Who may know about whom.** The sender remembers what it sent. Go's
   `videoNews`, which already sends each video's states one at a time,
   keeps the hash of each clip as it last sent it, and a state sends a
   clip whole only when its hash changed, and every clip by its key and
   its hash in order. The window's transport, `onVideoState` in
   `lib/api.ts`, keeps the clips it was sent and makes each state whole
   again, so nothing past it knows. Names that cross: `VideoState.Order`,
   `ClipMark` and `ClipEntry.Hash`, one binding to start over,
   `SendWhole`. Next changes: a new field of a clip is in its hash by
   itself. Cost: one Go file, one TypeScript file, their tests.
2. **What changes together.** Each clip carries the rev of the clock it
   last changed at, and a state leaves out the clips older than the
   window's. But a clip as the window sees it is made from its plan, the
   video's picture, the settings' crop width and the keyframes, so a rev
   would have to be stamped by every one of them, and one missed is a
   clip the window never hears changed. Only the clip as it is made
   tells whether it changed, so this one keeps the hash of 1 in place
   of a rev.
3. **Where state lives.** The window keeps the clips and asks for them:
   a state carries a hash of all its clips, and the window calls `Clips`
   when it differs. It breaks the rule that a card on its way becomes
   its clip in the same state that ends its job, #697, since the clips
   would come in a call of their own, and it doubles the trips exactly
   while a search writes clips, when states come fastest.
4. **What runs together.** A `clip` event for each clip that changed,
   beside the state. Two channels again, the problem the one state of
   #606 was made to end: the window could hold a clip from one moment
   and the jobs from another.
5. **What a test has to build.** Keep the state whole and make it
   smaller, the plan's path once instead of in every clip, the reason
   only for the clip chosen. Every field taken out is a change to every
   place that reads it, and 40 clips still do not fit in 8 KB.

## What converges

The state stays the one channel, 1, 2 and 5 keep it, 3 and 4 break it.
That a clip is known by what it holds and not by a stamp, from 2. That
the sender remembers and the transport undoes it, from 1, so the
session, the job store and every part that hears a state still get a
whole `VideoState`, the one shape it has. Dropped: 3 and 4 for the two
channels, 5 for not being enough, though it can follow on its own.

A window that lost track, opened again while Go went on, finds a key it
was not sent as it is now. It does not guess and does not read the
clips with a call of its own, which would be the second channel of 3:
it asks Go, `SendWhole`, to send the next state with every clip, and
takes nothing until a state it can make whole comes.

## The decision

```
interface   EpisodeSession, the job store: a whole VideoState, as before
  api.ts    onVideoState makes each state whole, lib/sent.ts,
            and asks SendWhole when it cannot
Go          videoNews.trim: Order, and only the clips whose hash changed
  State     reads the video's state whole, as before
```

A state read by a call, `State`, is whole and has no `Order`. Only a
state sent is trimmed.

## What follows

With 40 clips a state the window hears while a video is heard falls
from 45.6 KB to 4.7 KB, under the 8 KB Wails sends inline, so no fetch
and no queue behind one. With one clip changed it is 5.8 KB.

It costs Go a hash of every clip on every send. In a Go benchmark of
40 clips in a cloud session, five runs each, marshalling the state was
0.15 ms whole and is 0.24 ms trimmed with the hashes, 0.09 ms more four
times a second, off the window's thread, where the fetch Wails served
for each state over 8 KB is no longer made.

It would show wrong as a clip on screen that differs from its plan
until the next change, which is what a lost state would look like, or
as `SendWhole` in the log of a window that was never opened again.
