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
    name: "a label tells a video apart, the video keeps its name, and both are centred in the title bar",
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
      // Tim saw the label pill in the title bar a point higher than the
      // buttons, #399, twice, and then half a point low once it was
      // centred under the edge of the window alone. What he sees as the
      // bar runs from that edge to the line at its foot.
      ["label", ["nicolas", "Return"]],
      ["title bar"],
    ],
  },
  {
    // Plan row 2.181. Tim added a video and its clip timeline stayed bare
    // until its first clip came, where the waveform used to come within
    // seconds, measured on its own. The speech is held, so the search is
    // still hearing it when the waveform has to be there. Issue #759. Tim
    // looked at another video while the search of the one he had added
    // found its first clip, and when he went back its clip timeline had
    // no waveform until he clicked around. The clip timeline of the video
    // away from the screen took the clip and drew its waveform out of the
    // page, a pixel high and black. And the first clip came with no click.
    name: "a new video shows its waveform before its first clip, gets that clip with no click, and has the waveform after it came while the video was away",
    steps: [
      ["speech", "hold"],
      ["add", 120],
      ["waveform"],
      ["cards", 0],
      ["away", 20],
      ["speech", "go"],
      ["first clip before"],
      ["episode"],
      ["waveform"],
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
    // the way Remove with Delete takes it. Issue #673: it leaves at once,
    // also while the app stays in front, because the app watches the
    // folders of its videos. The other ways it is deleted, while the app
    // is away or closed, with the video, or written back after, are held
    // in Go, see letgo_test.go and reconcile_test.go, so this looks only
    // at the row leaving the sidebar.
    name: "a video whose folder is deleted outside the app leaves the list",
    steps: [
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "in front"],
      ["let go"],
    ],
  },
  {
    // Issue #311. Tim moved a video's file away in Finder and opened the
    // video again. A red line over the workspace said the file was gone
    // and pushed everything down, and a toast said it a second time. Put
    // back, the sidebar saw it and the video preview went on showing
    // nothing. Issue #673. A video moved away in Finder said File not
    // found only once the app came back to the front. Now the app watches
    // the folders of its videos, so the sidebar says it while the app
    // stays in front, and stops saying it once the file is back.
    name: "a video whose file is moved away says File not found at once while the app is in front, says so once in a toast when opened, and shows again once it is back",
    steps: [
      ["add", 20],
      ["wait for", "Search"],
      ["add", 20],
      ["wait for", "Search"],
      ["file moved in front"],
      ["file moved away"],
    ],
  },
  {
    // Issue #432, asked for by Tim: the transcript of the whole video is
    // exported from its menu with the word corrected in the caption box.
    name: "the transcript is exported with a word corrected in the caption box",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["transcript", "Text"],
      ["exported", "Schulhoff"],
      ["transcript", "Escape"],
      ["transcript", "Subtitles"],
      ["exported", "Schulhoff"],
      ["transcript", "click"],
      ["transcript", "keys"],
      ["exported", "Schulhoff"],
    ],
  },
  {
    // Issue #432: a video just added, still being heard by its first
    // search, is transcribed to its end from its menu, stopped and carried
    // on, and then offers its transcript.
    name: "a video added is transcribed to its end from its menu",
    steps: [
      // Its first search hears the first of its two windows and ends, so
      // half of it is still to come when Transcribe whole video is
      // clicked, and the click holds the speech so it stays to come.
      ["speech", "go"],
      ["add", 1500],
      ["transcript", "transcribe"],
      // A search started meanwhile stops it, for good.
      ["transcript", "search stops it"],
      // Asked for again, stopped and asked for again, each said in the
      // frame of the click.
      ["transcript", "continue"],
      ["transcript", "stop"],
      ["speech", "go"],
      ["transcript", "continue"],
      ["transcript", "stop at the end"],
      ["transcript", "Text"],
    ],
  },
  {
    // Tim's report of 26 October, 12.27: the whole video's transcription
    // and the work on its clips go one at a time.
    name: "Transcribe video waits until the search is done",
    steps: [["speech", "hold"], ["add", 1500], ["wait for", "Cancel"], ["transcript", "waits for the search"]],
  },
];
