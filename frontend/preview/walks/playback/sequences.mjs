// The sequences about the video preview: playing, looping, the frames it
// shows and the crop frame. They are cases found by hand, kept so they stay
// fixed. Their steps are said in steps.mjs beside this file, and in common/
// for the steps of every folder. See ../sequences.mjs.
export default [
  {
    // Tim's idea (2.269): paused, the frames of the word after the playhead
    // and of the word before are read while the hand rests, so Shift and an
    // arrow find their frame there. Before, a step to the next word read on
    // in the open stream, or opened one, while the hand waited.
    name: "a step to the word beside finds its frame read while the hand rested",
    steps: [
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["heard"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowLeft"],
      // Tim heard nothing when clips played on #229, so a play after the
      // frames were read ahead is listened to.
      ["heard"],
    ],
  },
  {
    // #396, from Tim: his podcast has two cameras, and a clip has one crop
    // across them, which only keyframes move. The search finds the clip of
    // two cameras in one piece, a cut across the switch and an edge
    // trimmed past it and put back leave it so, the crop dragged on the
    // first camera stands there on the second too, and the short shows
    // every frame through that one crop.
    name: "a clip filmed by two cameras has one crop, through a cut, a trim and a drag",
    steps: [
      ["add cameras", 30, 12],
      ["wait for", "Search"],
      ["pieces", 1],
      ["mark", "found"],
      ["look", "found"],
      ["one crop", "found"],
      ["cut switch", 1],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["framed", "found"],
      ["same", "found"],
      ["trim past", "end", 1],
      ["reset", "end"],
      ["framed", "found"],
      ["same", "found"],
      ["crop to", "left"],
      ["look", "moved"],
      ["one crop", "moved"],
      ["render"],
      ["shots", "moved"],
    ],
  },
  {
    // Bug 1 of docs/DESIGN-REVIEW.md: at 29.97 fps an edge kept to the
    // millisecond lies just before its frame's start for 1400 of every
    // 3000 frames, and the video preview drew the frame before the one the
    // short starts on. And a search left its edges between frames, where
    // the render took the frame whose start was nearer and the video
    // preview the frame that held the edge. The clip's edges here are the
    // search's own, its cuts are put by the hand, and at least one edge is
    // kept just before its frame's start. The picture starts after the
    // sound, so the frames are on a grid of their own, and the search's end
    // lies where the two rules used to part.
    name: "the video preview on a clip's edges shows the frames its short starts and ends on",
    steps: [
      ["add", 40, "30000/1001", "late"],
      ["wait for", "Search"],
      ["cards", 1],
      ["trim", "start", 20],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["edges as rendered"],
    ],
  },
  {
    // Plan row 2.198, found by Tim: a video OBS recorded in Matroska was
    // added, searched and found clips, and the video preview said "The
    // file has no moov box, so it is not an MP4 or MOV it can read". It
    // read the file's frames with a reader of its own, mp4.ts, where
    // everything else reads through ffmpeg. The picture starts after the
    // sound, by Matroska's first timestamp, so the frames are on a grid
    // of their own, as in the walk on a clip's edges above.
    name: "a video in Matroska shows in the video preview the frames its short has",
    steps: [
      ["add", 40, "30000/1001", "late", "mkv"],
      ["wait for", "Search"],
      ["cards", 1],
      ["trim", "start", 20],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["edges as rendered"],
    ],
  },
  {
    // Plan row 2.130: a phone records its frames at uneven times, which
    // the render counted as if they came every 1/29.97 s, and made the
    // short at the 50 frames a second ffmpeg reports for them. Now every
    // frame of the short is the frame the video preview shows for its
    // moment, at the frames' average rate rounded, 30.
    // TestARenderShowsTheFrameThatHoldsEachMoment holds the engine to it,
    // this the app.
    name: "a short of a phone's uneven frames shows what the video preview shows",
    steps: [
      ["add", 40, "30000/1001", "uneven"],
      ["wait for", "Search"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["as previewed", "30/1"],
    ],
  },
  {
    // Plan row 3.25: a phone turned while it recorded leaves a part of the
    // video lying on its side. The video here stands up by its display
    // matrix, so its frame number runs down its right side, and the part
    // after the cut, rotated a quarter to the left with R, reads across
    // its top again, in the video preview and in its short.
    // TestRotateTurnsThePartBetweenCutsAndFourTurnsBringItBack and
    // TestARotatedPieceIsRotatedBeforeItsCrop hold the engine to it, this
    // the app.
    name: "a part of a phone's video rotated with R stands up in the video preview and in its short",
    steps: [
      ["add", 30, "25", "turned"],
      ["wait for", "Search"],
      ["cards", 1],
      ["cut at", 0.5],
      ["cuts", 1],
      ["hold", "RotatePart"],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["render"],
      ["upright"],
    ],
  },
  {
    // Plan row 2.130: a picture that starts after its sound, by an edit
    // list. The clip's start is dragged to before the picture starts,
    // where the render took the picture's frames from its first on and the
    // sound from where the picture starts.
    name: "a short of a picture that starts after its sound shows what the video preview shows",
    steps: [
      ["add", 40, "30000/1001", "late"],
      ["wait for", "Search"],
      ["cards", 1],
      ["trim", "start", -60],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      // Plan row 2.137: edges counted from the start of the file stood
      // half a frame from the picture's own, which starts 0.25 s in.
      ["on frames"],
      ["render"],
      ["as previewed", "30000/1001", 0.5],
    ],
  },
  {
    // Tim saw the captions stay up in the video preview while the episode
    // played on through the chosen clip, with the clip dimmed on the clip
    // timeline. On the video the video preview shows the video alone, and
    // on the clip its crop frame and its captions. The clip starts with
    // the episode, so its start is trimmed first to leave room before it.
    name: "the episode played through a clip shows only the video, and the clip its frame and captions",
    steps: [
      ["trim", "start", 300],
      // From before, the play starts at 3.4 s and the clip at 7.0 s, and
      // the rule reads 2 s inside it from more than a frame on, 7.2 s at
      // this video's 5 fps. So the play has to reach 9.2 s, 5.8 s of play.
      // Six seconds left 0.2 s to spare, and the last frame the walk reads
      // comes behind the play, 0.18 s at SLOW=8, issue #519. Seven leave
      // 1.2 s.
      ["play from", "before", 7],
      ["only the video"],
      ["play from", "in", 4],
      ["the clip shown"],
    ],
  },
  {
    // The same with cuts in the clip, which Tim asked for: the episode
    // plays straight through the parts cut out and the pieces either side
    // of them, on the video the whole way, and the clip jumps its cuts,
    // on the clip the whole way. Neither may show the clip on the video,
    // in a piece or in a cut, nor lose it on the clip after a jump.
    name: "the episode played through a clip with cuts shows only the video, and the clip its frame and captions",
    steps: [
      ["trim", "start", 300],
      ["cut at", 0.25],
      ["cut at", 0.55],
      ["cuts", 2],
      ["play from", "before", 13],
      ["only the video", "across cuts"],
      ["play from", "in", 9],
      ["the clip shown", "across cuts"],
    ],
  },
  {
    // Tim played the episode from before the clip and pressed shift and
    // the right arrow to skip ahead, and the clip lit up the moment a step
    // landed in it. A step while the episode plays on the video skips
    // through the episode and stays on the video, cuts and all. On the
    // clip the same keys walk its words and the play stays on the clip.
    name: "shift and the arrows skip ahead through a clip on the video, and walk its words on the clip",
    steps: [
      ["trim", "start", 300],
      ["cut at", 0.4],
      ["cuts", 1],
      ["play from", "before", 8, "Shift+ArrowRight"],
      ["only the video"],
      ["play from", "in", 4, "Shift+ArrowRight"],
      ["the clip shown"],
    ],
  },
  {
    // Tim pressed L a few times while the clip played, and the play went
    // on to the clip's end and stood there, but the sound of the clip's
    // start played on for about three seconds. Loop switched off once the
    // play has worked out the next time through, in the last four seconds
    // of a time through, stops the play at the end, sound and picture.
    // Then Tim pressed L quickly near the end and heard clicks: that fix
    // started the play over on each press. A press of L touches nothing
    // that plays, so the sound runs on without a click.
    name: "loop switched off near the end stops the play at the clip's end",
    steps: [
      ["loop off near the end", 2],
      ["stopped at its end"],
    ],
  },
  {
    // Tim switched the loop on early in a clip with a cut, let it run, and
    // at the end the sound went round while the video preview stood on the
    // clip's last frame (2.264).
    name: "a clip looped from early on goes round its end, the picture with the sound",
    steps: [
      ["cut at", 0.85],
      ["loop through the end", 8],
    ],
  },
  {
    // The loop switched off with L as the sound crosses the clip's end,
    // while the page is held and the playhead has not caught up, ends the
    // play where the sound is, after one more time through. It ended the
    // play at the end the sound had just crossed, and the clip's start was
    // heard for a moment after it, on a busy machine (2.253).
    name: "loop switched off as the sound crosses the clip's end ends the play ahead of the sound",
    steps: [["loop off as the sound crosses the end"]],
  },
  {
    // Tim pressed the clip timeline while the clip played, to get a drag
    // ready, and the play ran on from the press at once, away from the
    // hand. A press holds the play under the hand, like a nail, until the
    // hand lets go, and the play goes on from there. Plan row 2.158.
    name: "a press on the clip timeline while it plays holds the playhead under the hand",
    steps: [["press while playing", -60], ["nailed"]],
  },
  {
    // Tim saw the episode's first frames flicker in the video preview as an
    // episode opened, and then the playhead jump to its clip. An episode
    // with a clip opens on the clip, and nothing before the clip is drawn
    // on the way. The clip starts later than the episode does, so a frame
    // of the episode's start is a frame that should not be there.
    name: "an episode opens on its clip, with no frame of its start on the way",
    steps: [
      ["trim", "start", 160],
      ["add", 30],
      ["episode watched"],
      ["opened on the clip"],
    ],
  },
  {
    // The video preview draws on a WebGPU canvas in the extended mode,
    // so an HDR episode shines on an HDR screen. Step 4 of 2.156.
    name: "the GPU draws standard video with QuickTime's look, a step for every value, and HDR as BT.2100's light",
    steps: [["screen light"]],
  },
  {
    // One engine: every frame comes from ffmpeg on the Go side, so a
    // webview with no decoder of its own plays the episode the same. Tim's
    // start.mp4, HEVC in 10-bit colour, was where it began: WebKit said it
    // would decode it and then failed on the first frame.
    name: "the picture comes from the Go side with no decoder in the browser",
    steps: [["no browser decoder"], ["from the app", 4]],
  },
  {
    // The video preview stayed black once the playhead had been to the end
    // of the episode, until the app started again: the cursor of the
    // episode's decoder that read to the end ended every stream after it
    // at once. Plan row 2.166.
    name: "the picture comes back after the playhead reached the end of the episode",
    steps: [["play to the end"], ["from the app", 4]],
  },
  {
    // A clip's end dragged past the end of the video was saved there, and
    // its short then came out shorter than the clip and failed its check.
    // On the bridge's video of 120 s the second drag saved 456.4 s. Plan
    // row 2.170.
    name: "a clip's end dragged past the end of the video stops where the video ends",
    steps: [["past the end"], ["from the app", 4]],
  },
  {
    // Tim saw the first frame of a workspace blurred until he played, and
    // sharp once he came back to it: it was decoded before the canvas was
    // told its size, at the 300 by 150 a canvas has before that, and kept
    // at it. Plan row 2.187.
    name: "a workspace opens on a sharp frame, and again after the app restarts",
    steps: [["sharp", "opened"], ["restart"], ["sharp", "opened"]],
  },
  {
    // The frame on screen while paused is asked for again when the app
    // grows. The bridge's picture is 320 by 180, smaller than the video
    // preview of the smallest app, so the app is made smaller than it can
    // be first, for a frame smaller than the picture to grow from.
    name: "a paused frame grows sharp with the app",
    steps: [["app size", 700, 500], ["restart"], ["sharp", "opened"], ["app size", 1500, 1000], ["sharp"]],
  },
  {
    // Tim's start.mp4 stood without a picture and would not play, on
    // Space or Play, while every other video was fine. It is HEVC in open
    // GOPs, and the playhead was on a frame shown before the key frame it
    // is decoded after: the episode's decoder started a key frame late,
    // and the frame the video preview waited for never came. Plan row
    // 2.186.
    name: "a frame shown before its key frame comes and plays",
    steps: [["add hevc"], ["leading frames"]],
  },
  {
    name: "the crop frame at either side of the picture is drawn whole, with the shade into its corners",
    steps: [
      ["app size", 1000, 1000],
      ["frame whole"],
      ["crop to", "left"],
      ["frame whole"],
      ["crop to", "right"],
      ["frame whole"],
      ["rotate at", 0.5],
      ["crop to", "left"],
      ["frame whole", "turned"],
    ],
  },
  {
    // Issue #539, Tim's ask: the number keys jump inside the chosen clip
    // the way they do in a video on YouTube, 0 to its start, 5 half way,
    // 9 to 90 %, playing or paused. Half way is of the clip as it plays,
    // so with a cut before the middle 5 lands past the cut. The cut is a
    // third of the way along, where it lies before the middle of what is
    // kept as long as it is under two fifths of the clip however wide the
    // app. A playhead on the video comes onto the clip, and a 5 typed in
    // a word stays there.
    name: "the number keys put the playhead that many tenths into the clip, past its cut, playing or paused",
    steps: [
      ["cut at", 0.3],
      ["cuts", 1],
      ["share", "5", "past the cut"],
      ["share", "9"],
      ["share", "0"],
      ["press", "Space"],
      ["share", "5"],
      ["press", "Space"],
      ["share", "9"],
      ["share", "5", "from the video"],
      ["share", "0"],
      ["click", "war"],
      ["press", "End"],
      ["share", "5", "in a word"],
    ],
  },
];
