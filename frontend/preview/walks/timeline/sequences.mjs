// The sequences about the clip timeline: edges, cuts, trims, marks,
// keyframes and the playhead under the hand. They are cases found by hand,
// kept so they stay fixed. Their steps are said in steps.mjs beside this
// file, and in common/ for the steps of every folder. See ../sequences.mjs.
export default [
  {
    // A paused video preview is held under the hand while an edge is
    // dragged, and cues its play only where the hand lets go. The play
    // after it starts with its sound.
    name: "a play after an edge dragged while paused is heard",
    steps: [
      ["drag edge", "end", -40],
      ["heard"],
    ],
  },
  {
    // Issue #755, found by Tim on youtube.mp4: dragging a clip's start
    // later, the captions over the video preview blinked off for a frame
    // each time the start passed a frame, and dragged earlier they did
    // not. The clip was handed to the video preview a frame before the
    // playhead went with the edge, and the frame on screen, out of the
    // clip by then, was drawn again with no captions. The same held for
    // its end dragged earlier.
    name: "the captions stay over the video preview while the clip's start is dragged later and its end earlier",
    steps: [
      ["watch captions"],
      ["drag edge", "start", 40],
      ["captions stayed"],
      ["watch captions"],
      ["drag edge", "end", -40],
      ["captions stayed"],
      ["watch captions"],
      ["drag edge", "start", 160],
      ["captions stayed"],
    ],
  },
  {
    // Issue #281, Tim's idea: while the playhead is dragged faster than
    // the frames under it can be decoded, the video preview shows the key
    // frame decoding starts from, the way established players scrub, and
    // the frame under the hand once it rests, never the key frame.
    name: "a playhead dragged far shows the key frame on the way, and the frame under the hand once it rests",
    steps: [["scrub far"], ["keyed on the way"]],
  },
  {
    // Issue #446. While a clip plays the playhead moves when the sound is in
    // another frame of the video, not on every animation frame: everything
    // that follows the playhead was worked out again 120 times a second on
    // Tim's screen for a video of 25 frames a second, and a play missed
    // refreshes for it.
    name: "a play moves the playhead once a frame of the video",
    steps: [["once a frame", 3]],
  },
  {
    // The same on HEVC with frames stored after their key frame, where the
    // decoder's way to a key frame differs most.
    name: "a playhead dragged far over HEVC shows the key frame on the way, and the frame under the hand once it rests",
    steps: [["add hevc"], ["scrub far"], ["keyed on the way"]],
  },
  {
    // A click outruns nothing, so it shows the frame clicked and no key
    // frame before it. The first version of #281 asked for the key frame
    // first for every place a press reached, the place it landed on too,
    // found by the review of #502.
    name: "a click far along the clip timeline shows the frame clicked and no key frame",
    steps: [["click far"]],
  },
  {
    // Plan row 2.235, found by Tim: the clip's edges took the playhead
    // along and a cut's edges left it where it was, so the frame beside
    // the cut could not be seen while it was placed.
    name: "a cut's edges dragged take the playhead along, the way the clip's edges do",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", -40],
      ["followed", "cut from"],
      ["drag edge", "cut to", 40],
      ["followed", "cut to"],
      ["drag edge", "end", -40],
      ["followed", "end"],
      ["drag edge", "start", 40],
      ["followed", "start"],
    ],
  },
  {
    // Found by Tim on youtube.mp4 (2.270): dragging a cut's left edge
    // into the clip, the video preview flickered between the edge's frame
    // and the frames after the cut. Each step of the drag cues the play
    // from the edge, which reads the next piece ahead, and the step after
    // withdrew those frames: their coming late was taken for a place the
    // hand had passed, and they were put up. And a playhead a hair before
    // the end of the piece was cued as inside it, while the cue's plan
    // started after the cut. At 25 frames a second, as Tim's video is, and
    // back and forth, because one drag catches it about half the time.
    name: "a cut's left edge dragged shows the frames before the cut, never one after it",
    steps: [
      ["added", 40, "25"],
      ["cut at", 0.5],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
    ],
  },
  {
    // Found by Tim on this: with shift, the playhead went inside the word
    // the edge landed on, a pause away from the edge, and dragging a cut's
    // left edge on past the last word took it over the cut into the
    // caption before. It stays in line with the edge.
    name: "a cut's edges dragged with shift keep the playhead in line with them",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", 40, "shift"],
      ["followed", "cut from"],
      ["drag edge", "cut from", -60, "shift"],
      ["followed", "cut from"],
      ["drag edge", "cut to", -40, "shift"],
      ["followed", "cut to"],
      ["drag edge", "cut to", 60, "shift"],
      ["followed", "cut to"],
      ["drag edge", "end", -60, "shift"],
      ["followed", "end"],
      ["drag edge", "start", 60, "shift"],
      ["followed", "start"],
    ],
  },
  {
    // Found by Tim: a press anywhere on the clip timeline moved the
    // playhead at once, and a press on an edge or a mark only when the
    // hand let go or began to move, so it caught up with a drag late.
    name: "a press on an edge or a mark puts the playhead on it at once",
    steps: [
      ["cut at", 0.5],
      ["thumbnail at", 0.25],
      ["keyframe at", 0.75],
      ["hold on", "end"],
      ["on press", "end"],
      ["hold on", "start"],
      ["on press", "start"],
      ["hold on", "cut from"],
      ["on press", "cut from"],
      ["hold on", "cut to"],
      ["on press", "cut to"],
      ["hold on", "thumbnail"],
      ["on press", "thumbnail"],
      ["hold on", "crop"],
      ["on press", "crop"],
      ["hold on", "caption start"],
      ["on press", "caption start"],
      ["hold on", "caption end"],
      ["on press", "caption end"],
    ],
  },
  {
    // Found by Tim: an edge let go of and taken again at once went to the
    // playhead standing on it. While the first drag was being saved the
    // edges were off the page.
    name: "an edge taken again at once after a drag is the edge's",
    steps: [
      ["cut at", 0.5],
      ["drag edge twice", "cut from", -30, -30],
      ["moved", "cut from"],
      ["followed", "cut from"],
      ["drag edge twice", "end", -30, -30],
      ["moved", "end"],
      ["followed", "end"],
    ],
  },
  {
    // An edge taken again before the last place the drag before it left
    // was answered starts from where that answer puts it, and the playhead
    // goes there with it. It started from the place answered before, a
    // frame behind, and the answer that came after the press put the edge
    // on its frame with the playhead left a frame away (#284).
    name: "an edge taken again before the drag before it is answered goes on from its answer",
    steps: [
      ["cut at", 0.5],
      ["drag edge twice", "cut from", -30, -30, 600],
      ["moved", "cut from"],
      ["followed", "cut from"],
      ["drag edge twice", "end", -30, -30, 600],
      ["moved", "end"],
      ["followed", "end"],
    ],
  },
  {
    // An edge taken again while the drag before it is still being saved
    // goes with the hand. It stood still until the save came back, which a
    // busy machine made long enough to see (2.259).
    name: "an edge taken again while the drag before it saves goes with the hand",
    steps: [
      ["cut at", 0.5],
      ["hold", "Reshape"],
      ["drag edge twice", "cut from", -30, -30],
      ["kept up", "cut from"],
      ["followed", "cut from"],
      ["hold", "Reshape"],
      ["drag edge twice", "end", -30, -30],
      ["kept up", "end"],
      ["followed", "end"],
    ],
  },
  {
    // A click on an edge and at once a press on the track: an answer for
    // the edge that came late put the playhead back on the edge.
    name: "a press on the track straight after a click on an edge keeps the playhead there",
    steps: [
      // The click once asked the engine where the edge went, and on a
      // long episode its answer came after the press. A click that does
      // not move the edge asks the Go side nothing now, so there is no
      // answer to hold, and the walk holds the playhead to the press.
      ["cut at", 0.5],
      ["edge then track", "cut from"],
      ["edge then track", "end"],
      ["edge then track", "start"],
    ],
  },
  {
    // Found by Tim: a cut's left edge pressed with shift and dragged right
    // first jumped left, to the nearest word, and only then went right.
    name: "an edge dragged with shift goes the way of the hand",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", 8, "shift"],
      ["went the hand's way", "cut from"],
      ["drag edge", "cut to", -8, "shift"],
      ["went the hand's way", "cut to"],
      ["drag edge", "end", -8, "shift"],
      ["went the hand's way", "end"],
      ["drag edge", "start", 8, "shift"],
      ["went the hand's way", "start"],
    ],
  },
  {
    // Found by Tim: a cut's edge dragged takes the playhead along, so it
    // stands on the edge, and the next press there took hold of the
    // playhead and not the edge.
    name: "a cut's edge with the playhead on it is the edge's to take",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", -40],
      ["drag edge", "cut from", -30, "at the playhead"],
      ["moved", "cut from"],
      ["followed", "cut from"],
    ],
  },
  {
    name: "a double-click in the clip cuts a part out, and one on the cut puts it back",
    steps: [
      ["mark", "start"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["same", "start"],
    ],
  },
  {
    // Tim swiped along the clip timeline with two fingers, clicked to put
    // the playhead where he was, and could not swipe on from there. The
    // playhead was then under the hand, and it was the one thing the
    // swipe did not go through.
    name: "a swipe goes on after a click put the playhead under the hand",
    steps: [
      ["swipe after click", 0.3, 60],
      ["swipe after click", 0.5, -60],
    ],
  },
  {
    name: "a double-click on a trimmed edge puts it back where the clip was found",
    steps: [
      ["mark", "start"],
      ["trim", "end", -120],
      ["reset", "end"],
      ["same", "start"],
      ["trim", "start", 90],
      ["reset", "start"],
      ["same", "start"],
    ],
  },
  {
    name: "undo takes back a trim and then a cut, and redo does them again",
    steps: [
      ["mark", "start"],
      ["cut at", 0.4],
      ["mark", "cut"],
      ["trim", "end", -100],
      ["mark", "trimmed"],
      ["undo"],
      ["same", "cut"],
      ["undo"],
      ["same", "start"],
      ["redo"],
      ["same", "cut"],
      ["redo"],
      ["same", "trimmed"],
    ],
  },
  {
    // A swipe along the clip timeline told the waveform's canvas its size
    // on every step, which throws its pixels away and makes it new, twice
    // the clip timeline's pixels on a Mac, with nothing changed in size.
    // Plan row 2.242.
    name: "a swipe along the clip timeline draws the waveform again on the same canvas",
    steps: [["swipe keeps the canvas", 12]],
  },
  {
    // Found by Tim on #188: dragging the playhead back over the cut before
    // a rotated part, the video preview turned the picture as the playhead
    // crossed the cut, before the frame from the other side had come, so a
    // frame stood turned the wrong way for a moment and the picture flipped
    // back and forth. And dragging the clip's end out past a rotated part
    // brought its frames back as filmed, not rotated.
    name: "a rotated part turns with its own frames under a dragged playhead, and stays rotated as its end moves",
    steps: [
      ["added", 30, "25", "turned"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["scrub", 0.95, 0.05],
      ["turned with its frames"],
      ["scrub", 0.05, 0.95],
      ["turned with its frames"],
      ["trim", "end", -60],
      ["trim", "end", 40],
      ["turned", [0, 90]],
    ],
  },
  {
    // A rotation belongs to the video's time, not to the cuts: putting back
    // the cut before a rotated part leaves the footage after the turn
    // rotated and the footage before it as filmed, and making the cut
    // again finds the part as it was. The review of #188 found the
    // rotation lost when it was kept on each piece.
    name: "a rotated part keeps its rotation when the cut before it is put back and made again",
    steps: [
      ["added", 30, "25", "turned"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["join", 1],
      ["cuts", 0],
      ["turned", [0, 90]],
      ["cut at", 0.5],
      ["cuts", 1],
      ["turned", [0, 90]],
    ],
  },
  {
    // Found by Tim testing #340, issue #348: rotating a part with R took
    // its keyframe diamond off the clip timeline, and the rotate mark
    // stood under the diamond. The keyframe stays through four turns, and
    // the rotate icon is stamped in the part's wash in the dark of the
    // empty track, large, in its middle, clear of the row the keyframes are in and of the row the
    // thumbnails are in. TestRotatingAPartKeepsItsKeyframes holds the
    // engine to it.
    name: "a rotated part keeps its keyframe, and has the rotate icon drawn in its middle",
    steps: [
      ["added", 30, "25", "turned"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["keyframe at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["keyframes", 1],
      ["stamp"],
      ["rotate at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["turned", [0, 0]],
      ["keyframes", 1],
    ],
  },
  {
    // Tim's camera switch, #750: one switch keyframe put with S where the camera
    // switches, and the crop dragged with the playhead on it. The crop
    // switches on the keyframe's own frame and holds, and the part before
    // keeps the clip's own crop, in the video preview frame by frame while
    // it plays, and after a cut, as the render crops it. Off the keyframe
    // the crop frame shows the crop and a drag moves nothing, Tim on #750.
    // The rule itself, two keyframes, a glide from one circle into the
    // next and a glide across a cut, is the engine's and tested there:
    // TestTheCropHoldsFromEachKeyframeToTheNext,
    // TestTheCropGlidesOnlyFromACircleIntoTheNext and
    // TestAGlideBetweenTwoCirclesRunsAcrossACut. The window only looks
    // the crop up in the steps Go sends,
    // TestTheVideoPreviewIsSentTheCropsTheRenderCropsBy holds those to
    // the render on every frame and keyframes.test.ts the lookup, so a
    // walk shows once that the video preview follows them, still,
    // playing and past a cut. A glide is no more steps to the window.
    // A keyframe before a cut once stopped holding its crop after it,
    // found by Tim, plan row 2.238.
    name: "one switch keyframe put with S switches the crop there, holds it past a cut, and the crop is placed only with the playhead on it",
    steps: [
      ["added", 30, "25"],
      ["hold", "SetCropMark"],
      ["crop before", 0.25],
      ["keyframe at", 0.5],
      ["drag crop", 300],
      ["crop kept", 0.25],
      ["crop before", 0.75],
      ["playhead at", 0.25],
      ["drag crop", -150],
      ["crop kept", 0.25],
      ["crop kept", 0.75],
      ["keyframes", 1],
      ["crop holds"],
      ["crop holds while playing"],
      ["cut at", 0.75],
      ["cuts", 1],
      ["crop holds across the cut"],
      ["under the hand", "crop"],
      ["remove mark", "crop"],
      ["keyframes", 0],
    ],
  },
  {
    // Found by Tim: a thumbnail and a keyframe lit by the playhead stood
    // half a pixel and a pixel to the right of its line. Plan row 2.234.
    name: "a thumbnail and a keyframe on the playhead's frame stand centred on its line",
    steps: [
      ["added", 30, "25"],
      ["thumbnail at", 0.1],
      ["keyframe at", 0.1],
      ["on the playhead", 2],
      ["thumbnail at", 0.9],
      ["keyframe at", 0.9],
      ["on the playhead", 2],
      ["pinch in"],
      ["on the playhead", 2],
    ],
  },
  {
    // Found by Tim on #192: C put no mark on a clip's first or last frame,
    // only one frame in. Found by Tim on #752: on a video whose picture
    // starts 4 ms after its sound, the way OBS records, the video preview
    // showed the frame before the playhead's, so S was not offered on the
    // clip's first frame and a keyframe was never the one shown.
    name: "a keyframe goes on a clip's first and last frame with S, and comes off with S",
    steps: [
      ["added", 30, "25", "late4ms"],
      ["keyframe on", "last"],
      ["keyframe on", "first"],
    ],
  },
  {
    // Tim's glide, #750: G puts a circle, and two circles are joined by a
    // line along the top rule as soon as the second is put, whatever their
    // crops. S on a circle makes it a diamond and the line goes, G a
    // circle again and the line comes back, and G on a circle removes it.
    // The line follows the start edge while the hand still holds it, found
    // by Tim on #750 with a cut's edge.
    // The first circle has no crop of its own, so a drag of the crop on
    // it moves nothing: the glide starts from the crop before it, Tim on
    // #778.
    // Found on #192: Automatic crop took the keyframes of the angle away
    // with the crop, and Put the crop back brought back the crop alone,
    // so what the keyframes did was lost to one click. Each comes back
    // with its crop and its kind. Where the crop glides is the engine's,
    // see the walk of S above.
    name: "two circles put with G are joined by a glide line that, with the first circle, follows the clip's start while it is dragged, S and G change their kind, and Put the crop back after Automatic crop brings them back",
    steps: [
      ["added", 30, "25"],
      ["glide at", 0.2],
      ["crop before", 0.2],
      ["drag crop", 400],
      ["crop kept", 0.2],
      ["glide at", 0.8],
      ["glides", 1],
      ["drag crop", 400],
      ["glides", 1],
      ["keyframe key", ["s", "diamond"]],
      ["glides", 0],
      ["keyframe key", ["g", "circle"]],
      ["glides", 1],
      ["glide under the hand", 0.5],
      ["crop before", 0.8],
      ["button", "Automatic crop"],
      ["keyframes", 0],
      ["button", "Put the crop back"],
      ["keyframes", 2],
      ["glides", 1],
      ["crop kept", 0.8],
      ["keyframe key", ["g", "none"]],
      ["keyframes", 1],
    ],
  },
  {
    // Found by Tim on #192: a keyframe dragged past the clip's end went a
    // pixel past the clip edge while held and jumped back to the last
    // frame when let go. It was drawn on the middle of the frame under the
    // hand while held and on the start of the frame once saved.
    name: "a keyframe dragged past the clip's edge stops on its first or last frame and stays where it was let go",
    steps: [
      ["added", 30, "25"],
      ["keyframe at", 0.5],
      ["keyframe past", "end"],
      ["keyframe past", "start"],
    ],
  },
  {
    // Issue #535, found by the review of #529: the clip timeline took a
    // second press within half a second and six pixels along it for a
    // double-click, whatever its height, so a press on the track and then
    // on the keyframe over it at the same x removed the keyframe.
    name: "a press on the track and then on the keyframe over it is no double-click",
    steps: [
      ["added", 30, "25"],
      ["keyframe at", 0.5],
      ["press under, then on the keyframe"],
    ],
  },
  {
    // Plan row 2.190, found by Tim on #188: a thumbnail mark did not
    // react to the pointer, and was removed only by putting the playhead
    // on it and pressing T. It is the keyframe's kind of mark, lit under
    // the hand and removed with a double-click, the way a cut is.
    name: "a thumbnail mark lights under the hand and a double-click removes it",
    steps: [
      ["added", 30, "25"],
      ["thumbnail at", 0.5],
      ["thumbnail at", 0.2],
      ["under the hand", "thumbnail"],
      ["remove mark", "thumbnail"],
    ],
  },
  {
    // Found by Tim on #201: with a word open in the caption box, a click
    // on a thumbnail's camera or a keyframe's diamond moved the frame and
    // left the caption box on the word's moment, since the marks take no
    // focus and the word kept it.
    name: "a click on a thumbnail or a keyframe while a word is open ends the correction, and the captions follow",
    steps: [
      ["added", 30, "25"],
      ["thumbnail at", 0.1],
      ["keyframe at", 0.9],
      ["open a word, then the mark", "thumbnail"],
      ["open a word, then the mark", "crop"],
    ],
  },
  {
    // Found on #201: on a busy machine the walk "an episode opens on its
    // clip" broke, because the hand let go of the clip's start before the
    // engine's answer to where it was let go had come, and the playhead
    // stayed on a frame the trim had cut away.
    name: "an edge let go before the engine answers leaves the playhead on the edge",
    steps: [
      ["answers late"],
      ["trim", "start", 160],
      ["playhead on", "start"],
    ],
  },
  {
    // Tim saw the play stutter, the playhead with it, as he dragged the
    // edge of a cut while the clip played. Only the space bar and a click
    // on the clip timeline change a play: an edge dragged while it plays
    // is saved and drawn, and the play goes on as it was.
    name: "an edge dragged while the clip plays changes nothing that plays",
    steps: [
      ["cut at", 0.6],
      ["cuts", 1],
      ["drag while playing", "cut", 40],
      ["played through"],
      ["drag while playing", "end", -60],
      ["played through"],
    ],
  },
  {
    // Issue #277, decided by Tim: an edge moved while the clip plays
    // changes nothing that plays. An end dragged back past the playhead
    // leaves the playhead outside the clip, on the video, so the play goes
    // on as a play of the video, past where the clip ended before. Paused,
    // the playhead goes with the edge, see "a cut's edges dragged take
    // the playhead along".
    name: "an end dragged back past the playing playhead plays on into the video",
    steps: [["end back past the playhead", 0.5]],
  },
  {
    // Tim on #277: he pulled the end of the clip in, played it, and put
    // the end back with Cmd+Z while it played, and the play still stopped
    // at the end he had taken back. A play plays the clip as it is now.
    name: "an end pulled in and undone while the clip plays plays on to the old end",
    steps: [["end in and back", 0.75, "paused", "undo"]],
  },
  {
    // The same with the end pulled in while it plays, and Redo after the
    // Undo: the play stops at the end pulled in.
    name: "an end pulled in while the clip plays, undone and redone, stops at the new end",
    steps: [["end in and back", 0.75, "playing", "undo", "redo"]],
  },
  {
    // Found by the review of #568: a change anywhere in the clip started
    // the play again while the playhead was in the last two seconds
    // before a cut, where the decoders already worked on the piece after
    // it. The play takes in only what the change reaches.
    name: "an end pulled in and undone while the clip plays just before a cut plays on to the old end",
    steps: [
      ["cut at", 0.5],
      ["cuts", 1],
      ["end in and back", 0.8, "before a cut", "undo"],
    ],
  },
  {
    // Tim on #277: a cut taken away by Undo while the clip plays is
    // played through, and put back by Redo it is jumped.
    name: "a cut undone and redone while the clip plays is played through and then jumped",
    steps: [
      ["cut at", 0.6],
      ["cuts", 1],
      ["menu while playing", "undo", 0.4],
      ["menu while playing", "redo", 0.4],
    ],
  },
  {
    // Tim moved the crop to the side of the picture and saw the viewer's
    // rounded corner cut into the crop frame's line, and bright specks of
    // the picture in the frame's own rounded corners, where the shade
    // beside it stopped short of them. An app this size makes the picture
    // fill the viewer, so its corners are the viewer's. Plan row 2.195.
    // Found by Tim on #192: the clip's frame on the clip timeline stood
    // just inside the track's border, with the grey of the border showing
    // above and below it, and the window's bars on the range picker stood
    // just outside it, with the border showing under them.
    name: "the clip's frame and the window's bars lie on their track's borders",
    steps: [["on borders"]],
  },
  {
    // Issue #361, asked for by Tim: the row under the clip timeline said
    // the chosen clip's title and numbers, which its card says, and
    // nothing said where the playhead was.
    name: "the playhead's time is in a field beside the buttons, and the clip's numbers on its card",
    steps: [["time field"], ["card says"], ["cut at", 0.5], ["card says"], ["card follows", -60], ["card says"]],
  },
  {
    // Issue #596, found by Tim on 29 October: a slow drag of the playhead
    // stuttered where a fast one did not. To the left, every step back
    // was a stream of its own, a seek, and the picture stood for each.
    name: "a slow drag of the playhead to the left reads its frames in runs, not a seek a step",
    steps: [["slow drag left", 40]],
  },
  {
    // Issue #377, found by Tim on 19 October: outside a clip, Shift and
    // the right arrow walked the words heard and stopped dead where the
    // transcript ended, in the middle of the video. Outside a clip there
    // is no caption to step through, so Tim chose a skip of 5 seconds,
    // heard or not. The speech is held, so the end of what was heard
    // stays where it is and no search finds a clip meanwhile.
    name: "Shift and the arrows skip 5 seconds outside a clip, past where the transcript ends",
    steps: [["speech", "hold"], ["add", 600], ["heard some", 3], ["skip outside a clip"], ["speech", "go"]],
  },
  {
    // Tim on #677: a play turns the clip timeline's page as the playhead
    // reaches its edge, and Shift and an arrow outside the clip skipped
    // out of the view and left it behind.
    name: "Shift and an arrow outside a clip turn the clip timeline's page, the way a play does",
    steps: [["skips in sight", 4]],
  },
  {
    // Found by the review of #677: a cut is inside the clip, and Shift and
    // an arrow there walk to the next word. With the 5 s skip outside a
    // clip, a press in a cut skipped 5 s, and while the clip played left it.
    name: "Shift and an arrow in a cut of the clip walk to the next word, not 5 seconds",
    steps: [
      ["cut at", 0.5],
      ["shift in a cut"],
    ],
  },
];
