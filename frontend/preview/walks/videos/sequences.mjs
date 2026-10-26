// The sequences about the videos in the sidebar: added, dropped, labelled
// and gone from the disk. They are cases found by hand, kept so they stay
// fixed. Their steps are said in steps.mjs beside this file, and in common/
// for the steps of every folder. See ../sequences.mjs.
export default [
  {
    // Found by Tim testing #385: going back to a video from the sidebar,
    // the video preview went black for a frame before its picture came
    // back, on the video whose frame had captions. What lies over the
    // frame was drawn again before the woken video preview had a frame of
    // its own, and drawn over nothing.
    name: "a video gone back to from the sidebar keeps its frame until it is drawn again",
    steps: [
      ["add cameras", 120, 60, "hevc"],
      ["wait for", "Search"],
      ["wake on a caption"],
    ],
  },
  {
    // Tim's videos are each youtube.mp4 in a folder of their own, and the
    // sidebar showed the same name for all of them. A label, words and a
    // colour from the three dots of a row, tells them apart, and the video
    // keeps its name.
    name: "a label tells a video apart and the video keeps its name",
    steps: [
      // A second video, so the list has the sort mark at its head.
      ["add", 12],
      ["label", [null, "Escape"]],
      ["label", [null, "Return"]],
      // Escape in the colour picker of a video with no label leaves it
      // with none, with a shade clicked and without.
      ["label", [null, "the colour picker, Escape"]],
      ["label", [null, "a colour of its own, Escape"]],
      ["label", ["Tab", "Escape"]],
      ["label", ["Tab", "Return"]],
      ["label", [null, "a colour of its own, Escape"]],
      ["label", [null, "a colour of its own, Return"]],
      ["label", [null, "Remove label"]],
      ["label", [null, "Blue"]],
      ["label", ["Ben, Kindergarten", "Return"]],
      ["label", ["Anna", "Escape"]],
      ["label", ["Dora", "Return, the save failing"]],
      ["label", ["", "Return"]],
      ["label", ["Clara", "Green after a press"]],
      // Tim clicked a colour on a label and pressed Return, and the menu
      // stayed open with a ring round it.
      ["label", [null, "Blue, then Return"]],
      ["two menus"],
    ],
  },
  {
    // Tim saw the label pill in the title bar a point higher than the
    // buttons, #399, twice, and then half a point low once it was
    // centred under the edge of the window alone. What he sees as the
    // bar runs from that edge to the line at its foot.
    name: "the name of a video and its label are centred in the title bar",
    steps: [
      // A second video, so the sidebar has the sort mark the label step
      // starts from.
      ["add", 12],
      ["label", ["nicolas", "Return"]],
      ["title bar"],
    ],
  },
  {
    name: "a new video gets its first clips with no click",
    steps: [
      ["add", 60],
      ["wait for", "Search"],
      ["cards", 1],
    ],
  },
  {
    // Plan row 2.181. Tim added a video and its clip timeline stayed bare
    // until its first clip came, where the waveform used to come within
    // seconds, measured on its own. The speech is held, so the
    // search is still hearing it when the waveform has to be there.
    name: "a video just added shows its waveform before its first clip",
    steps: [
      ["speech", "hold"],
      ["add", 120],
      ["waveform"],
      ["cards", 0],
      ["speech", "go"],
      ["wait for", "Search"],
      ["cards", 1],
    ],
  },
  {
    // Step 3 of docs/VIDEO-PREVIEW.md: Add asks the episode's decoder for a
    // file's first frame, and leaves out a file with no picture, with the
    // reason, where before it was taken and the video preview failed on it.
    name: "a file with sound and no picture is left out by Add, with the reason",
    steps: [["add sound alone"], ["left out", "nur-ton-"]],
  },
  {
    // Plan row 2.192. Tim asked for a video dropped anywhere on the app,
    // whatever page it is on, to do what Add does: it is added, it opens,
    // and its first search finds its clips with no click.
    name: "a video dropped on any page is added and opens, the way Add adds one",
    steps: [
      ["drop", 30, "settings"],
      ["dropped in front"],
      ["wait for", "Search"],
      ["cards", 1],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["dropped in front"],
      ["drop again", "updates"],
      ["dropped in front"],
      ["no reason"],
    ],
  },
  {
    // Issue #327. Tim dropped a picture on the app and a red line
    // over the workspace pushed it all down. What went wrong with nothing
    // on screen about it is a toast that moves nothing, goes by itself
    // after a while the way toasts do, stays while the pointer is on it,
    // and goes at once by its cross.
    name: "a file that is not a video is said in a toast that moves nothing and goes by itself unless held",
    steps: [
      ["hold the clock"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast goes"],
      ["hold the clock"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast held"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast closed"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["drop not a video", ["workspace", "zweite"]],
      ["not a video"],
      ["stacked"],
      ["toast closed"],
      ["toast closed"],
      ["no reason"],
    ],
  },
  {
    // Plan row 2.247. Tim deleted a video's folder in Finder to remove the
    // video, and it stayed in the sidebar with no clips and no lamp, and
    // opening it wrote a folder back. Its folder is everything the app
    // has of a video, so a video whose folder is gone has left the list,
    // the way Remove with Delete takes it.
    name: "a video whose folder is deleted outside the app leaves the list",
    steps: [
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "away"],
      ["let go"],
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "closed"],
      ["let go"],
      // Tim then deleted two videos from Documents with their folders,
      // and they stayed with a red lamp, File not found.
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "with the video"],
      ["let go"],
      // One of them had been opened after it was deleted, which wrote its
      // chosen clip back, and the app opened on it after the update and
      // said it was not in the library.
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "written back"],
      ["let go"],
    ],
  },
  {
    // Issue #311. Tim moved a video's file away in Finder and opened the
    // video again. A red line over the workspace said the file was gone
    // and pushed everything down, and a toast said it a second time. Put
    // back, the sidebar saw it and the video preview went on showing
    // nothing.
    name: "a video whose file is moved away says so once, in a toast, and shows again once it is back",
    steps: [
      ["add", 20],
      ["wait for", "Search"],
      ["add", 20],
      ["wait for", "Search"],
      ["file moved away"],
    ],
  },
];
