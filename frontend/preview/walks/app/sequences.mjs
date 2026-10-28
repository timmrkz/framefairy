// The sequences about the app as a whole: its size, its menus, the settings
// and the Updates page. They are cases found by hand, kept so they stay
// fixed. Their steps are said in steps.mjs beside this file, and in common/
// for the steps of every folder. See ../sequences.mjs.
export default [
  {
    // Plan row 2.189. Cmd+Z and Cmd+Y come from the app's menu, which
    // the workspace answers with its own undo. On every other page they go
    // to the field being typed in, which did nothing before Report a
    // Problem needed it, in the settings too.
    name: "Undo and redo in a settings field undo and redo the typing",
    steps: [
      ["settings"],
      ["field", "input.licence-key", "FF-12"],
      ["undo"],
      ["field reads", "input.licence-key", ""],
      ["redo"],
      ["field reads", "input.licence-key", "FF-12"],
    ],
  },
  {
    // Issue #339: the licence key field and Unlock stood half a pixel
    // down, their top at 699.5, because the words beside them were an odd
    // height, and so they would step as they faded or moved.
    name: "the licence row stands on whole pixels, on one line and on two",
    steps: [
      ["settings"],
      ["licence row whole"],
      ["licence row whole", "refused"],
    ],
  },
  {
    // Tim's report of 2026-10-27, 17:21: the settings called clips
    // shorts, drew the speech model and the clip model two ways, offered
    // a size for some models and "best here" for one, spoke of the models
    // in six places, listed every caption font by name in one row and
    // wrote paths three ways.
    name: "The settings say each thing once, in one way",
    steps: [["app log", "open"], ["settings"], ["models alike"], ["said once"]],
  },
  {
    // Tim saw the names and the licences in Help, Acknowledgements sit on
    // the border of their box, with no room round them (#500). Each group
    // is now a card the way the settings draw one.
    name: "Acknowledgements keeps its words in from the edges of its cards",
    steps: [["acknowledgements"]],
  },
  {
    // Plan row 2.245. Settings, Advanced shows the app's log, and its bin
    // removes every file of it, the older one too, after asking. Cancel
    // leaves it as it was, and the log that follows is the one new file.
    name: "The app's log is removed whole from Settings, after asking",
    steps: [
      ["app log", "open"],
      ["settings"],
      ["app log", 2],
      ["button", "Remove the app's log"],
      ["button", "Cancel"],
      ["app log", 2],
      ["button", "Remove the app's log"],
      ["button", "Remove them"],
      ["app log", 1],
    ],
  },
  {
    // Issue #370. Tim pressed Escape on the Font list and the list let
    // go with no ring, on the Style list and its trigger kept the ring,
    // and in the Size field and nothing happened. Escape lets go of
    // every list, pop-up and field the same way, everywhere in the app.
    name: "Escape lets go of every list, pop-up and field with no ring left",
    steps: [
      ["escape lets go", "#caption-font"],
      ["escape lets go", "#caption-style"],
      ["escape lets go", "label.setting:has-text('Size') input"],
      ["escape lets go", "button.colour >> nth=0"],
      ["settings"],
      ["escape lets go", "input.licence-key"],
    ],
  },
  {
    // Tim had the pointer on the sidebar and swiped up for Mission Control,
    // and saw the sidebar open and close over and over until he clicked
    // back into the app.
    name: "The sidebar stays shut under a pointer that does not move, and while the app is not in front",
    steps: [["mission control"]],
  },
  {
    // Tim chose another model in the settings while a search ran, and
    // nothing stopped him. What a search uses stays as it is until it is
    // done, and once it is, it can be changed again.
    name: "a model a search finds clips with is not changed or removed under it",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["settings"],
      ["pick model"],
      ["refused", "finding"],
      ["remove model", "language"],
      ["refused", "finding"],
      ["remove model", "speech"],
      ["refused", "speech"],
      ["episode"],
      ["head"],
      ["wait for", "Continue"],
      ["settings"],
      ["remove model", "language"],
      ["asks"],
      ["pick model"],
      ["changed"],
    ],
  },
  {
    // The same from the first moment of a search, while it still hears a
    // video just added and has not come to the model yet.
    name: "a model is not changed or removed while a search still hears",
    steps: [
      ["speech", "hold"],
      ["add", 120],
      ["wait for", "Cancel"],
      ["settings"],
      ["pick model"],
      ["refused", "finding"],
      ["remove model", "speech"],
      ["refused", "speech"],
      ["hearing"],
    ],
  },
  {
    // Tim recorded the range picker sawing up and down while he dragged
    // the app's edge, then the settings fields shaking: a height rounded
    // in steps of three left the picture the rest, and the columns beside
    // it took what the picture left, on a half pixel every other step.
    name: "the app dragged a pixel at a time moves every part one way, the sides equal and set by the width",
    steps: [["resize"]],
  },
  {
    // Issue #248: the app from its smallest to a large screen, a Studio
    // Display and a Pro Display XDR at the size macOS gives them by
    // default. The resize above holds every part to one way while the app
    // is dragged between 960 and 1700 wide, this holds the whole of it at
    // each size.
    name: "the workspace fits the app from its smallest size to a large screen, nothing cut off and no strip of nothing",
    steps: [
      ["fits the app", [960, 640]],
      ["fits the app", [1280, 800]],
      ["fits the app", [1728, 1117]],
      ["fits the app", [2560, 1440]],
      ["fits the app", [3008, 1692]],
    ],
  },
  {
    // Issue #248: three features in one history. A crop moved, a word
    // corrected and a font picked are taken back in turn and done again in
    // order. The rules hold every Undo and Redo to the clip before and
    // after its step, its crop and how its captions look with its words.
    name: "undo takes back a crop moved, a word corrected and a font picked, in turn, and redo does them again",
    steps: [
      ["mark", "start"],
      ["drag crop", 200],
      ["differs from", "start"],
      ["mark", "cropped"],
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["differs from", "cropped"],
      ["mark", "corrected"],
      ["font", "Archivo Black"],
      ["differs from", "corrected"],
      ["mark", "font"],
      ["undo"],
      ["same", "corrected"],
      ["undo"],
      ["same", "cropped"],
      ["undo"],
      ["same", "start"],
      ["redo"],
      ["same", "cropped"],
      ["redo"],
      ["same", "corrected"],
      ["redo"],
      ["same", "font"],
    ],
  },
  {
    // Issue #248: a restart with work in every state. A clip trimmed, its
    // short rendered and a search stopped are as they were once the app
    // opens again: the search's row with Continue, the clip's pieces on
    // the clip timeline and its card, and the short beside it.
    name: "a restart brings back a clip trimmed, its short and a search stopped, as they were",
    steps: [
      ["mark", "start"],
      ["trim", "end", -100],
      ["differs from", "start"],
      ["mark", "trimmed"],
      ["render"],
      ["rendered"],
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["same", "trimmed"],
      ["card says"],
      ["rendered"],
      ["model", "answers"],
    ],
  },
  {
    // Found by Tim on #192: in a narrow app the buttons under the clip
    // timeline gave way first and stood taller than they were wide.
    name: "the buttons under the clip timeline stay square in a narrow app",
    steps: [["square buttons"], ["app size", 900, 800], ["square buttons"]],
  },
  {
    // Tim made a build with make install, and its Updates page followed
    // pull request 143, picked by a build before it, and said it was not
    // checked yet with a Check that would never look by itself.
    name: "a build made on the Mac follows nothing until a channel is chosen, and then follows it",
    steps: [
      ["updates", { stored: "pr-143", list: "main,pr-143" }],
      ["channel", "Choose a channel"],
      ["choices", ["Branch main", "PR #143"], ""],
      ["status", "Built on this Mac", "It stays as it is until you choose a channel to follow."],
      ["rail mark", "none"],
      ["no check"],
      ["follow", "Branch main"],
      ["at once", "Looking for a newer build"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
      ["channel", "Branch main"],
      ["choices", ["Branch main", "PR #143"], "Branch main"],
    ],
  },
  {
    name: "a build from a channel lists its channels and no Nothing",
    steps: [
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready"],
      ["rail mark", "dot, accent"],
      ["choices", ["Branch main", "PR #20"], "Branch main"],
    ],
  },
  {
    name: "a pull request followed that closed says closed in the list and in the line under it",
    steps: [
      ["updates", { from: "main", stored: "pr-143", list: "main" }],
      ["status", "PR #143 is closed", "Nothing downloads until you choose what to follow next."],
      ["build mark", "triangle, warning"],
      ["rail mark", "dot, warning"],
      ["warned"],
      ["channel", "PR #143, closed"],
      ["choices", ["Branch main", "PR #143, closed"], "PR #143, closed"],
    ],
  },
  {
    // Tim checked with Wi-Fi off and the line under the build gave the Go
    // error as the reason, an address and a port over three lines (#417).
    name: "a check that cannot reach GitHub says so in words, and what the system said copies",
    steps: [
      ["updates", { from: "main", list: "main", offline: true }],
      ["status", "The check did not get through", "Could not reach GitHub. Check your internet connection."],
      ["rail mark", "dot, failure"],
      ["update detail", "The channel list could not be fetched"],
    ],
  },
  {
    // Tim asked for a pull request in conflict with main to be in the list
    // and say so, the way a closed one does. Its build is of the branch as
    // it is, so it can still be followed and installed.
    name: "a pull request in conflict with main says so in the list and can still be followed",
    steps: [
      ["updates", { from: "main", list: "main,pr-20,pr-21", conflict: "pr-20" }],
      ["choices", ["Branch main", "PR #21", "PR #20, conflicts"], "Branch main"],
      ["fits"],
      ["follow", "PR #20, conflicts"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
      ["channel", "PR #20, conflicts"],
      ["build mark", "dot, warning"],
      ["rail mark", "dot, warning"],
      ["warned"],
    ],
  },
  {
    // Tim could relaunch into another build while a clip search ran, which
    // stops the search half way (#382). Relaunch waits for it, following
    // another channel meanwhile stops nothing, and once the search ends
    // Relaunch can be pressed again by itself.
    name: "Relaunch waits while a search runs, and the search goes on",
    steps: [
      ["speech", "hold"],
      ["add", 120],
      ["wait for", "Cancel"],
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready", "Relaunch once the search is done"],
      ["relaunch", "waits"],
      ["follow", "PR #20"],
      ["status", "A newer build is ready", "Relaunch once the search is done"],
      ["relaunch", "waits"],
      ["hearing"],
      ["speech", "go"],
      ["relaunch", "ready"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
    ],
  },
  {
    // Tim stepped through the first run on 27 October and asked for one
    // page: a card for each model, nothing downloading until he pressed
    // Download, both at once, rows that read as a table, and no button
    // to press once both were here. The app then opens on its first page
    // with one big button to add a video.
    name: "The first run downloads both models side by side and opens on the page to add a video",
    steps: [["first run"]],
  },
];
