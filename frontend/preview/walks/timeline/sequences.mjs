// The sequences about the clip timeline: edges, cuts, trims, marks,
// keyframes and the playhead under the hand. They are cases found by hand,
// kept so they stay fixed. Their steps are said in steps.mjs beside this
// file, and in common/ for the steps of every folder. See ../sequences.mjs.
export default [
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
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
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
      ["add", 30, "25", "turned"],
      ["wait for", "Search"],
      ["cards", 1],
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
      ["add", 30, "25", "turned"],
      ["wait for", "Search"],
      ["cards", 1],
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
      ["add", 30, "25", "turned"],
      ["wait for", "Search"],
      ["cards", 1],
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
    // Plan row 3.26: keyframes put with C, the second dragged to another
    // place, glide the crop from the one to the other, in the video
    // preview frame by frame while it plays as the render crops it.
    // TestTheRenderGlidesTheCropOverTheMarks holds the render to the same
    // glide, and glide.cases.json the window and the engine to each other.
    name: "keyframes put with C glide the crop from one to the next in the video preview, frame by frame",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["hold", "SetCropMark"],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["glides"],
      ["glides while playing"],
      ["under the hand", "crop"],
      ["remove mark", "crop"],
      ["keyframes", 1],
    ],
  },
  {
    // Found by Tim: a keyframe before a cut and the next one after it did
    // not glide, the crop stood still on both sides. Plan row 2.238.
    name: "a keyframe before a cut glides to one after it",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["cut at", 0.5],
      ["cuts", 1],
      ["glides across the cut"],
    ],
  },
  {
    // Found by Tim: a thumbnail and a keyframe lit by the playhead stood
    // half a pixel and a pixel to the right of its line. Plan row 2.234.
    name: "a thumbnail and a keyframe on the playhead's frame stand centred on its line",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
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
    // only one frame in.
    name: "a keyframe goes on a clip's first and last frame with C, and comes off with C",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["keyframe on", "last"],
      ["keyframe on", "first"],
    ],
  },
  {
    // Found on #192: Automatic crop took the keyframes of the angle away
    // with the crop, and Put the crop back brought back the crop alone,
    // so the glide was lost to one click.
    name: "Put the crop back after Automatic crop brings back the keyframes and their glide",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["button", "Automatic crop"],
      ["keyframes", 0],
      ["button", "Put the crop back"],
      ["keyframes", 2],
      ["glides"],
    ],
  },
  {
    // Found by Tim on #192: a keyframe dragged past the clip's end went a
    // pixel past the clip edge while held and jumped back to the last
    // frame when let go. It was drawn on the middle of the frame under the
    // hand while held and on the start of the frame once saved.
    name: "a keyframe dragged past the clip's edge stops on its first or last frame and stays where it was let go",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["keyframe at", 0.5],
      ["keyframe past", "end"],
      ["keyframe past", "start"],
    ],
  },
  {
    // Plan row 2.190, found by Tim on #188: a thumbnail mark did not
    // react to the pointer, and was removed only by putting the playhead
    // on it and pressing T. It is the keyframe's kind of mark, lit under
    // the hand and removed with a double-click, the way a cut is.
    name: "a thumbnail mark lights under the hand and a double-click removes it",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
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
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
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
      ["answers late", 500],
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
      ["press", "Space"],
      ["drag while playing", "end", -60],
      ["played through"],
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
];
