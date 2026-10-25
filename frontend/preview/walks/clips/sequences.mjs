// The sequences about the clip list: cards, clips made, removed and put
// back, and the shorts rendered from them. They are cases found by hand,
// kept so they stay fixed. Their steps are said in steps.mjs beside this
// file, and in common/ for the steps of every folder. See ../sequences.mjs.
export default [
  {
    // Putting a clip back takes back its remove and nothing more. Tim
    // played a clip, removed another and put it back, and the one put
    // back was chosen and played from its start. Issue #323.
    name: "a clip put back while another plays leaves the play alone",
    steps: [["remove clip"], ["make here", "i"], ["put back"], ["put back another while playing"]],
  },
  {
    // A clip is always chosen while there is one: removing the chosen clip
    // chooses the next, or the one before it at the end of the list, and
    // putting it back chooses it again. Tim found nothing chosen. Issue
    // #331.
    name: "removing the chosen clip chooses the one beside it",
    steps: [
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["remove the chosen", "first"],
      ["put back"],
      ["chosen again"],
      ["remove the chosen", "last"],
    ],
  },
  {
    // The clip chosen when it was removed is the one exception: putting it
    // back takes the remove back as it was, chosen, and the playhead stays.
    name: "the clip chosen, removed and put back, is chosen again where the playhead was",
    steps: [["put back the chosen"]],
  },
  {
    name: "a word corrected in one clip reads as it was heard in another clip over it",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["clips with", ["Schulhoff", 1]],
      ["clips with", ["Schulhof", 1]],
    ],
  },
  {
    name: "a word removed in one clip is still shown in another clip over it",
    steps: [
      ["frame", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["clips with", ["Schulhof", 1]],
    ],
  },
  {
    // Found by Tim on 20 October: the edge of a clip's card in the
    // transcript curved round its corners, where the clip list's is a
    // straight bar the corners cut off. Both are one bar, a pixel wider.
    name: "the edge of a clip's card is the same in the transcript and the clip list",
    steps: [["edges alike"]],
  },
  {
    // Found in the second review of the search on 21 October: two clips
    // over the same words have a card each that begin at the same moment,
    // and Shift and the down arrow stuck on the first, because the step to
    // the second left the playhead where it was.
    name: "Shift and the arrows walk past two cards that begin at the same moment",
    steps: [["walk two at once"]],
  },
  {
    // Found by Tim on 20 October: the clip chosen with the keys had its
    // mark on the range picker and its frame on the clip timeline a step
    // darker than the hand on its card made them, and a clip's card under
    // the hand was a step darker than the chosen one.
    name: "the hand and the keys light a clip the same",
    steps: [["one light", "clips"]],
  },
  {
    // Found by Tim on 20 October: Shift and the arrows brought a clip
    // walked to just into view, so the walk went on at the foot of the
    // list. They turn the page, the way the transcript does as it plays.
    name: "Shift and the arrows turn the page of the clip list",
    steps: [["walk the clip list"]],
  },
  {
    // Issue #383, found by Tim on a video of hours: a clip card clicked
    // moved the crop frame at once and took the captions off the picture,
    // the clip's picture came a few frames later and its captions a few
    // after that. All three change together, in one frame, also when the
    // Go side is slow to give the clip's captions. Two cameras, so the
    // clips have their crops far apart, in HEVC, which the Go side decodes.
    name: "a clip card clicked changes the picture, the crop frame and the captions in one frame",
    steps: [
      ["add cameras", 120, 60, "hevc"],
      ["wait for", "Search"],
      ["pick cards", 8],
    ],
  },
  {
    // Issue #383, found by Tim on a video of almost three hours: a clip
    // card clicked for the first time took half a second to put up its
    // clip, the frame decoded from the key frame before it and its
    // captions asked of the Go side after the click.
    name: "a clip card clicked puts up its clip from what was read while the hand rested",
    steps: [
      ["add cameras", 120, 60, "hevc"],
      ["wait for", "Search"],
      ["pick rested cards", 6],
    ],
  },
  {
    // Plan row 2.177. Tim added a video of 15 s with Shortest at 20 s:
    // its first search failed, the range picker had no line on it and the
    // clip timeline showed a minute.
    name: "a video shorter than a clip is its own clip, with rulers that fit it",
    steps: [
      // Held, so the rulers are looked at with no clip yet, the
      // way Tim saw them once the search had failed.
      ["speech", "hold"],
      ["add", 15],
      ["rulers fit", 15],
      ["speech", "go"],
      ["wait for", "Search"],
      ["cards", 1],
      ["whole clip", 15],
      ["rulers fit", 15],
    ],
  },
  {
    // Tim removed the one clip of a video of 15 s and pressed Search, and
    // every search failed with "there is nothing to make a clip of in
    // this video": the clip removed still counted as the one moment the
    // video has. Search brings it back, and Search with it in the list finds
    // nothing more, fails nothing either, and says so where its clip
    // would have appeared.
    name: "a video shorter than a clip gets its clip again once it was removed",
    steps: [
      // The row that closes by itself has its time run on by hand, so
      // the app's timers are Playwright's from the start, see fakeClock.
      ["fake clock"],
      ["add", 15],
      ["wait for", "Search"],
      ["whole clip", 15],
      ["remove clip"],
      ["head"],
      ["wait for", "Search"],
      ["whole clip", 15],
      // The clip made again is the clip that was removed, so the removed
      // one is not offered back, and undoing its removal does not bring
      // it back beside the new one. Both showed the video twice.
      ["no row", "Nothing new found"],
      ["no put back"],
      ["undo refused"],
      ["whole clip", 15],
      ["head"],
      ["wait for", "Search"],
      ["whole clip", 15],
      // Tim saw the row of that search close again with nothing to say.
      // It says so now, as news: it runs down like a removed clip's row
      // and closes.
      ["row closes", ["Nothing new found", 10]],
      // Search again is a new search, and it says so again.
      ["head"],
      ["wait for", "Search"],
      ["row closes", ["Nothing new found", 10]],
    ],
  },
  {
    // Tim removed a clip, worked on in the workspace, and the time to put
    // it back stood still, the fill and all, until his hand passed over
    // the row again. On his Mac a row moved out from under a hand that
    // stood still is never told the hand is off it, and the time waited
    // for that word. Plan row 2.240.
    name: "a row to put back runs down once the hand is elsewhere, though it moved from under a still hand",
    steps: [["trim", "start", 300], ["make while paused", "i"], ["remove two, hand still"]],
  },
  {
    // Tim removed clips with the bin, and once a row's time ran out it
    // closed to a thin strip and then the list jumped up the rest in one
    // frame: the space between the rows stayed until the row was gone.
    // Plan row 2.275.
    name: "a removed row closes with the space after it, in one movement",
    steps: [["trim", "start", 300], ["make while paused", "i"], ["row folds"]],
  },
  {
    // Found while 2.124 was proven: from a 29.97 fps episode every piece
    // of a short had a frame too many and about 30 ms of padded silence at
    // each cut, because a piece went to ffmpeg rounded to the millisecond.
    // TestARenderCutsOnWholeFrames holds the engine to it, this the app.
    name: "a render of a 29.97 fps episode cuts on whole frames",
    steps: [
      ["add", 40, "30000/1001"],
      ["wait for", "Search"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["short", 2],
    ],
  },
  {
    // Found while 2.125 was proven: with a folder for shorts named in the
    // settings, which the bridge does, a clip never learnt it was
    // rendered, because only the episode's own out folder was looked in.
    // Render went on saying Render and Show in folder never came. After
    // a restart too, because where the short went is kept on disk.
    name: "a short rendered into the folder for shorts is known as rendered",
    steps: [["render"], ["rendered"], ["by its words"], ["restart"], ["rendered"]],
  },
  {
    // Issue #384: Tim rendered a clip, uploaded the short and removed it
    // and its pictures from the folder. The clip lost its green dot and its
    // green mark, as if it had never been rendered.
    name: "a clip stays rendered after its short is removed from its folder",
    steps: [["thumbnail"], ["render"], ["rendered"], ["short removed"], ["restart"], ["rendered, short gone"]],
  },
  {
    // Plan row 2.133, found while 2.127 was fixed: a short is named after
    // its clip, and two episodes render into the one folder for shorts the
    // bridge names, so the clip of a second episode with the same name
    // wrote over the first episode's short, and the first episode's clip
    // went on saying it was rendered, over the other episode's short.
    name: "two episodes never overwrite each other's shorts",
    steps: [
      ["thumbnail"],
      ["render"],
      ["pictured"],
      ["keep", "first"],
      ["words again"],
      ["add", 120],
      ["wait for", "Search"],
      ["cards", 1],
      ["render"],
      ["apart", "first"],
      ["rendered"],
    ],
  },
  {
    // Plan row 2.133: two episodes of the same file name share their
    // folder in the folder for shorts, so the second short of the same
    // name gets a number, the way Finder gives one.
    // The first clip has two pictures and the second none, so a render of
    // the second that went by the clip's name would take them away.
    name: "two episodes of one file name number their shorts",
    steps: [
      ["thumbnail"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["thumbnail"],
      ["render"],
      ["pictured"],
      ["keep", "first"],
      ["words again"],
      ["add namesake", 120],
      ["wait for", "Search"],
      ["cards", 1],
      ["render"],
      ["apart", "first", " 2"],
      ["rendered"],
    ],
  },
  {
    // Plan row 2.133: Render again writes over the clip's own short, in
    // its place and under its name, with its pictures beside it, and
    // nothing else in the folder.
    name: "render again writes the short over in place, with its pictures beside it",
    steps: [
      ["thumbnail"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["thumbnail"],
      ["render"],
      ["rendered"],
      ["pictured"],
      ["keep", "first"],
      ["render"],
      ["in place", "first"],
      ["rendered"],
      ["pictured"],
    ],
  },
  {
    // Tim played the episode and pressed I to make a clip of what he was
    // watching, and the clip came up dimmed: no gesture had put the
    // playhead on it, so it counted as on the video. A clip made with I
    // or O is made around the playhead, so the playhead is on it from the
    // frame it is there, on its way and once it is written, and the play
    // goes on as it was.
    name: "a clip made with I while the episode plays is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while playing", "i"],
      ["made on the clip"],
    ],
  },
  {
    name: "a clip made with O while the episode plays is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while playing", "o"],
      ["made on the clip"],
    ],
  },
  {
    // Paused, the playhead is put on the clip's start while it is on its
    // way, and it was dimmed there too until it was written.
    name: "a clip made with I while the video is paused is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while paused", "i"],
      ["made on the clip"],
    ],
  },
  {
    // Plan row 2.211: Tim saw the highlight in the video preview with
    // padding round the word, settled, where the short had it mid-bounce
    // and the thumbnail had it with no padding at all. A browser drew the
    // preview's captions and libass the short's. Now libass draws both,
    // from the one caption file, see lib/frames/captions.ts.
    name: "the video preview shows the short's captions, frame for frame",
    steps: [["render"], ["captions like the short"]],
  },
];
