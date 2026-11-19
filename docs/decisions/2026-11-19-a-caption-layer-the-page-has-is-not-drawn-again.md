# A caption layer the page has is not drawn again

Issue #896.

## The problem

The issue, from the speed hunt of 16 November: "Between two words the
caption layer over the video preview does not change, but libass draws
it and the bridge carries it again for every frame." It asked for this:
"The page asks with the caption, word and scale it has, and Go answers
that the layer is unchanged without drawing it. A boundary change, so the
five designs come first."

Read on the branch, 19 November:

- `CaptionLayers.fetch` in `frontend/src/lib/frames/captions.ts` asks
  `/frames/captions` for a layer for every frame of a playing clip, the
  frame drawn and up to three ahead, `GOING`.
- `serveCaptions` in `cmd/framefairy-app/captions.go` probes the video,
  takes its decoder and has libass draw the layer, `PreviewCaptions`,
  every time.
- The caption file holds no animation. Every event has a fixed start and
  end to the hundredth and fixed tags: the pop of a word is one event a
  frame, each at its own scale, see `popSpans` in `engine/highlight.go`.
  So what libass draws at a moment is set by which events are on then,
  and by the size of the short there.
- Measured in the cloud session against the bridge in Chromium, a video
  of 1920 by 1080 at 25 frames a second, its first clip played for 6.5 s
  at 1500 by 1000, 6 runs: about 385 layers a run, 183 of them the same
  bytes as the layer before, 47 %.

## Five simulations

### 1. Who may know about whom

Only Go knows when the caption file changes: the events are its, and so
are the runs of words lit and the pop. The page knows a layer it holds
and nothing of why it looks the way it does. So Go names each layer by
what it is drawn from, and the page hands the name back with the next
ask. Go works out the name of the frame asked for without drawing it,
and when the two are the same, answers with nothing. The page keeps the
layer it has. Crossing the boundary: one header on the answer, the tag,
and one on the ask, the tag it has. The page holds no rule, it echoes a
string. This is what HTTP's `ETag`, `If-None-Match` and 304 do.

### 2. What runs together

Better than an ask answered with nothing is no ask. Go could say with
each layer the part of the episode's clock it holds for, and the page
would use it for every frame inside without asking. That removes the
round trip too, about a millisecond on the bridge. But the page then
decides which frame falls in which part: a comparison of frame times
in floats against bounds Go worked out on the short's clock, cut at
every piece of the clip, where the short's size may change. The frontend
holds no rules, and an off-by-one frame there leaves a word lit over a
frame that lights the next. Dropped. What comes back: the page must not
decide sameness itself, Go must.

### 3. Where state lives

Go could keep the last layer it drew for each video, by the same name,
and answer the next ask with those bytes from memory, without libass.
That saves the drawing but not the sending, the page's reading of the
bytes or its copy to the screen's texture. And the bytes are already in
the page, so it is a second copy of state the page has. Dropped. What
comes back: the layer the page holds is the one to keep, and the screen
skips uploading pixels it holds already, `holds.words` in `screen.ts`,
so the page must keep the same layer object, not a copy.

### 4. What changes together

The name has to change exactly when the drawing could. That is when an
event of the caption file starts or ends, and those moments are made in
one place, from the same `litRuns` and `popSpans` that write the file
and that `PopScale` and `ShowingAt` already read. So the moments go
beside them in the engine, `CaptionDrawing.Steady`, and a change to how
the file is written is tested against it: the test reads the file's own
events back and holds that none starts or ends inside a span `Steady`
gives. The short's size per piece goes into the engine too,
`CaptionDrawing.ShortSize`, which `PreviewCaptions` uses, so the name
and the drawing read one rule. The app adds the rest of the ask to the
name, track, drafts, part, typing and size, by hashing every parameter
but the frame.

### 5. What a test has to build

The engine's moments: a clip with words a frame long, words lit
together, a pop on every word and a gap, its caption file made by the
engine and read back, no decoder. The route: the committed picture
fixture and the decoder, as the route's other tests use, asking frame by
frame with the tag of the layer before and checking that every frame
answered unchanged would have drawn those very bytes. The page: Vitest
with a stand-in fetch, checking that the tag sent is the newest layer's,
never one of other captions or another size, and that a frame answered
unchanged keeps the same layer object.

## What converges

1, 3, 4 and 5 keep the decision in Go and the layer in the page, and 2
is dropped for putting a rule about frames in the page. 4 puts the
moments in the engine next to what writes the file, 3 has the page keep
the very object it has. From 1 the convention, which first pointed at
`ETag`, `If-None-Match` and 304. Wails v3.1.0 takes `If-None-Match` off
every request on Windows, `request_windows.go` in its `assetserver`,
because WebView2 hangs on a 304, and Windows is a platform this app is
for. So the shape is the convention's, with headers of our own and 204
No Content for the answer with nothing.

Weighed after the review: libass says itself whether a frame differs
from the one it drew before, `detect_change` from `ass_render_frame`.
But it says so after it has laid the frame out, and of the frame the
same decoder drew last, which is not the frame the page holds once
three are asked at a time or another clip asked in between. And the
answer would come from the decoder, a child program, so the request
still waits for it. The moments come from the caption file's own times
in Go, before anything is drawn.

## The decision

```
page   CaptionLayers.fetch     asks with X-Caption-Have: the tag of the
                               layer that came last, same captions, same size
                               204: keeps that layer for the frame
app    serveCaptions           layerTag: the ask but the frame, the steady
                               moments, the short's size, hashed
                               same as X-Caption-Have: 204, nothing drawn
                               else draws, and sends X-Caption-Tag
engine CaptionDrawing.Steady   the moments around a frame over which the
                               caption file starts and ends nothing
       CaptionDrawing.ShortSize the short's size on a frame
```

No new part, home for state or boundary: `/frames/captions` stays the
one way the page asks for a layer, and `CaptionLayers` the one place it
keeps them. `docs/ARCHITECTURE.md` is unchanged. `docs/CAPTIONS.md`
says how the tag works.

## What follows

- A playing clip has 39 % fewer layers drawn by libass, 31 % fewer bytes
  carried, and the page waits 38 % less on them, measured as above.
- A frame just after a word changes, asked while the layer of that word
  was still on its way, brings the older tag and is drawn again. That is
  about one in six of the frames that are the same, the price of asking
  ahead.
- What would show it wrong: a word lit or a pill grown on a frame of the
  video preview that the short does not show there. The engine test reads
  the caption file's events back for that, and the route test compares
  the bytes of every frame answered unchanged.
