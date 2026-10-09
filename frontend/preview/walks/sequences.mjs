// Named sequences of steps with what has to come of them: the cases found
// by hand, kept so they stay fixed. Every step is also checked against the
// rules every walk is, see rules.mjs. See docs/TESTING.md.
//
//   BRIDGE_URL=http://127.0.0.1:8123/ node sequences.mjs [part of a name]
//
// A step is a list, its name first:
//
//   ["frame", word]    walks the playhead with Shift and → until the word is
//                      in the caption box, clicks it and leaves it with
//                      Escape, so it wears the keyboard's frame
//   ["click", word]    clicks the word, which opens it
//   ["press", key]     presses a key, as Playwright names it
//   ["type", text]     types
//   ["undo"], ["redo"] the menu bar's Undo and Redo
//   ["mark", label]    remembers the engine's captions and pieces
//   ["cut at", share]  double-clicks in the clip, this share of the way
//                      along it, which cuts a part out there
//   ["join", n]        double-clicks on the clip timeline's nth cut, from 1,
//                      which puts the part back
//   ["trim", edge, px] drags the clip's "start" or "end" edge by px
//   ["reset", edge]    double-clicks the clip's edge, which puts it back
//                      where the clip was found
//   ["cut switch", n]  double-clicks on the clip timeline where the clip's
//                      nth shot meets the next, from 1, which cuts a part
//                      out across the camera switch
//   ["trim past", edge, n]
//                      drags the clip's "start" or "end" edge two seconds
//                      past the nth camera switch, so the shot beyond it is
//                      gone
//   ["model", how]     the language model "holds" its answers, "fails" or
//                      "answers"
//   ["add", seconds]   adds a new video of so many seconds with Add
//   ["add", seconds, rate]
//                      adds one filmed at a frame rate, "30000/1001" for
//                      29.97 fps, whose frames keep their numbers in a
//                      short
//   ["add", seconds, rate, timing]
//                      the same with its frames at a phone's uneven times,
//                      "uneven", or its picture starting after its sound,
//                      "late"
//   ["render"]         presses Render and waits until the short is written
//   ["add cameras", seconds, switch]
//                      adds one filmed by two cameras that switch so many
//                      seconds in, each looking at its own subject
//   ["look", label]    remembers, for every piece of the clip, the crop
//                      frame in the video preview with the playhead in the
//                      middle of the piece, put there with a click on the
//                      clip timeline: where it stands and what it shows
//   ["head"]           presses the clip list's head button, New, Cancel or
//                      Continue
//   ["hold", call, ms] the next call of this name waits so many
//                      milliseconds before it reaches the Go side, the way
//                      a busy machine delivers it late
//   ["new and cancel"] presses New, and Cancel in the first frame the head
//                      says it, and every frame from New to that press the
//                      head says Cancel and takes the click, never greyed
//                      out
//   ["restart"]        closes the app and opens it again on the episode
//   ["speech", ms]     the speech model takes so many milliseconds over
//                      each piece of audio, so a video added is heard
//                      slowly enough to do something while it is
//   ["settings"]       opens the settings from the sidebar, and remembers
//                      the models they name
//   ["episode"]        opens the episode on screen before again
//   ["episode watched"]
//                      the same, reading back from the video preview every
//                      frame it draws for the episode from the click on
//   ["pick model"]     opens the list of what finds clips and, if it
//                      opens, picks the last model here it does not have
//                      picked
//   ["remove model", which]
//                      presses the bin of the "language" model on this
//                      machine, under Downloaded models, or of the
//                      "speech" model
//   ["decoder fails"]  makes the browser's picture decoder fail on its first
//                      frame, after saying it takes the file, the way
//                      WebKit's does with HEVC in 10-bit colour, and opens
//                      the episode again
//   ["add namesake", seconds]
//                      adds a new video of so many seconds with the file
//                      name of the bridge's episode, from another folder
//   ["thumbnail"]      presses T, which makes the frame under the playhead
//                      a thumbnail of the clip, and waits for the clip to
//                      have one more
//   ["keep", label]    remembers the clip on screen, its short and its
//                      pictures on disk, and the shorts in its folder
//   ["updates", how]   opens the Updates page from the sidebar on a build
//                      as how says: from the channel how.from, or made on
//                      the Mac without it, with how.stored picked by a
//                      build before it, and the channels how.list on the
//                      list, each with a newer build
//   ["follow", name]   picks the channel of this name in the Updates
//                      page's list, and reads the page in the next frame
//   ["words again"]    the speech stand-in says its sentences again from
//                      the first word, so a video added next is heard as
//                      the bridge's episode was and its clip has the same
//                      name
//
// and what has to come of them:
//
//   ["opened on the clip"]
//                      since "episode watched", the video preview drew the
//                      first frame of the chosen clip and no other
//   ["box", text]      the caption box reads this, word by word
//   ["open", word]     this word is open for typing
//   ["same", label]    the engine's captions and pieces are what they were
//                      at the mark
//   ["cuts", n]        the clip timeline shows n cuts
//   ["wait for", word] the clip list's head button comes to say this, in a
//                      minute at most
//   ["row", words]     a row of the clip list says this
//   ["cards", n]       the clip list holds n clips
//   ["spans", label]   every caption appears and goes when it did at the mark
//   ["pieces", n]      the clip timeline draws the clip in n pieces
//   ["framed", label]  the clip timeline draws as many pieces as at the
//                      look, and at each moment looked at the crop frame
//                      stands where it stood and shows what it showed
//   ["shots", label]   the short, read back from disk: every frame shows
//                      what the crop frame showed on its shot at the look,
//                      and at every camera switch the sound runs straight
//                      on, no 10 ms of it quieter than 80% of the tone
//   ["from the app", n]
//                      the picture comes from the Go side, and n clicks
//                      along the clip timeline each show the frame that
//                      holds the playhead
//   ["on frames"]      every edge of the clip the hand put, all but its
//                      end and any before the picture starts, lands on
//                      one of the picture's own frames, counted from where
//                      the picture starts, the frames the render cuts on
//   ["resize"]         drags the app's height and then its width a pixel at
//                      a time, down to the app's smallest, and on every step every part of the
//                      workspace moves only one way the whole drag, stands
//                      on a whole pixel, and meets the app's edges, and the
//                      two sides beside the picture, measured from the
//                      app's edges, are the same width, or a pixel apart,
//                      and stand still while only the height changes
//   ["short", cuts]    the clip has at least so many cuts, none of them on
//                      a frame that starts on a whole millisecond, and the
//                      short Render wrote, read back from disk, holds
//                      exactly the frames of the clip's pieces, each the
//                      right frame of the episode, with its sound as long
//                      as its picture and quiet at a cut for no longer
//                      than the render's fade
//   ["apart", label]   the clip on screen, of another episode with a clip
//                      of the same name as the one kept, has a short of its
//                      own, the short kept is still the same file, and the
//                      clip kept still says it is its short
//   ["apart", label, number]
//                      the same, with the short on screen called as the
//                      one kept with this after it, " 2". With both, the
//                      pictures kept are all still there, untouched
//   ["in place", label]
//                      the clip's short is the one kept, under the same
//                      name with no number, written again since, and no
//                      other short appeared in its folder
//   ["by its words"]   the clip's short is named after the clip's words
//                      alone, <slug>.mp4, with nothing of its id
//   ["pictured"]       the short's pictures, one for each thumbnail of the
//                      clip, lie beside it in its episode's folder, and no
//                      picture more
//   ["as previewed", rate]
//                      the short Render wrote, read back from disk, has
//                      this frame rate and as many frames as the clip's
//                      pieces last, and at every cut, at the clip's two
//                      ends and on every tenth frame between, its frame is
//                      the frame the video preview shows at the moment that
//                      frame stands for, read off the canvas with the
//                      playhead put there by a click on the clip timeline
//   ["edges as rendered"]
//                      the short Render wrote, read back from disk, and the
//                      video preview agree on the clip's edges: with the
//                      playhead put on the clip's start and on where each
//                      cut ends by a click on it, the video preview shows
//                      the frame the short starts that piece with, and on
//                      the clip's end the short's last frame. The clip's
//                      end is where the search put it, and at least one
//                      edge is kept to the millisecond just before its
//                      frame's start
//   ["as previewed", rate, before]
//                      the same, for an episode whose picture starts after
//                      its sound: the clip's first piece starts before
//                      this moment, inside the time before the picture
//   ["refused", card]  the gesture before it shook the "finding" or the
//                      "speech" card and opened no list and no box, the
//                      settings name the models they named at "settings",
//                      and with "finding" the line under Find clips with
//                      says a search is finding clips with it
//   ["asks"]           the gesture before it opened the box asking whether
//                      to remove, which Escape closes again
//   ["changed"]        the settings name another model than at "settings"
//   ["hearing"]        a search still hears, so what came before was
//                      about a search that had not come to the model yet
//   ["rendered"]       the clip says its short is in its episode's folder
//                      inside the folder the settings name for shorts, the
//                      file is there and the app serves it, Render says
//                      Render again and Show in folder is there
//   ["channel", name]  the Updates page's list says this, in ten seconds
//                      at most
//   ["choices", names, picked]
//                      the list offers these channels and nothing else, in
//                      this order, with this one picked, or none with ""
//   ["status", head, more]
//                      the line under the build comes to say this head, in
//                      ten seconds at most, and more somewhere after it
//   ["no check"]       the line under the build offers no button
//   ["at once", head]  in the frame after the pick, the list said the
//                      channel picked and the line under it this head
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, realpathSync, statSync } from "node:fs";
import { basename, dirname, extname, join } from "node:path";
import { open, chosen, shown, engineState, handles, timeline, middle, high, drag } from "./bridge.mjs";
import { clipList, control, pressHead, fromSidebar, settle, ask, episodeOn, shortFrames, shortSound } from "./bridge.mjs";
import { xOf, seekTo, cropFrame, sameLook, sayLook, readShort, loudness, filmedOnScreen, preview } from "./bridge.mjs";
import { Watch, same, describe } from "./rules.mjs";

export const sequences = [
  {
    name: "delete removes the framed word and nothing else",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["box", "Ich war sechs Jahre alt,"],
      ["spans", "start"],
    ],
  },
  {
    name: "a word saved empty is removed",
    steps: [
      ["click", "Jahre"],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs alt,"],
    ],
  },
  {
    name: "undo puts a removed word back and redo takes it again",
    steps: [
      ["mark", "start"],
      ["frame", "sechs"],
      ["press", "Delete"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    name: "a removed word typed back after the word before it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "war"],
      ["press", "End"],
      ["type", " vielleicht"],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a removed word typed back before the word after it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "sechs"],
      ["press", "Home"],
      ["type", "vielleicht "],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a word typed in beside another is a word of its own, removed alone",
    steps: [
      ["mark", "start"],
      ["click", "war"],
      ["press", "End"],
      ["type", " noch"],
      ["press", "Enter"],
      // A word more takes room, so the last word goes on to the next
      // caption. Adding may move words, removing may not.
      ["box", "Ich war noch vielleicht sechs Jahre"],
      ["click", "noch"],
      ["open", "noch"],
      ["press", "Escape"],
      ["press", "Backspace"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["same", "start"],
    ],
  },
  {
    name: "Enter opens the framed word, wherever the playhead is",
    steps: [
      ["click", "Jahre"],
      ["press", "Escape"],
      ["press", "Enter"],
      ["open", "Jahre"],
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
    name: "a new video gets its first clips with no click",
    steps: [
      ["add", 60],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    name: "Cancel stops a search, and Continue carries it on to its clips",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    // Plan row 2.131. Cancel is pressed before the Go side has heard of
    // the search New asked for, which is held on its way there, so the
    // Cancel reaches the Go side first. The search never starts, and says
    // Stopped with Continue where its clips would have been.
    name: "Cancel pressed at once after New stops the search before it starts",
    steps: [
      ["hold", "Search", 1500],
      ["new and cancel"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
    ],
  },
  {
    name: "a search the app was closed on says Interrupted, after it opens again",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Interrupted. Click Continue"],
    ],
  },
  {
    name: "a search that failed says so, and Continue tries again",
    steps: [
      ["model", "fails"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Failed. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
    ],
  },
  {
    // Tim's steps for #116, on a clip a search parted at a camera switch
    // into two pieces, each with the crop of its own shot.
    name: "a cut or an edge put back keeps the camera switch, and the short runs straight through it",
    steps: [
      ["add cameras", 30, 12],
      ["wait for", "New"],
      ["pieces", 2],
      ["mark", "found"],
      ["look", "found"],
      ["cut switch", 1],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["framed", "found"],
      ["same", "found"],
      ["trim past", "end", 1],
      ["pieces", 1],
      ["reset", "end"],
      ["framed", "found"],
      ["same", "found"],
      ["render"],
      ["shots", "found"],
    ],
  },
  {
    // Found while 2.124 was proven: from a 29.97 fps episode every piece
    // of a short had a frame too many and about 30 ms of padded silence at
    // each cut, because a piece went to ffmpeg rounded to the millisecond.
    // TestARenderCutsOnWholeFrames holds the engine to it, this the app.
    name: "a render of a 29.97 fps episode cuts on whole frames",
    steps: [
      ["add", 40, "30000/1001"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["short", 2],
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
      ["wait for", "New"],
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
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["as previewed", "30/1"],
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
      ["wait for", "New"],
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
    // Found while 2.125 was proven: with a folder for shorts named in the
    // settings, which the bridge does, a clip never learnt it was
    // rendered, because only the episode's own out folder was looked in.
    // Render went on saying Render and Show in folder never came. After
    // a restart too, because where the short went is kept on disk.
    name: "a short rendered into the folder for shorts is known as rendered",
    steps: [["render"], ["rendered"], ["by its words"], ["restart"], ["rendered"]],
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
      ["wait for", "New"],
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
      ["wait for", "New"],
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
    // The same from the first moment of a search, while it still hears a
    // video just added and has not come to the model yet.
    name: "a model is not changed or removed while a search still hears",
    steps: [
      ["speech", 3000],
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
    name: "removing a word in the middle of a caption leaves the caption whole",
    steps: [
      ["mark", "start"],
      ["frame", "auf"],
      ["press", "Backspace"],
      ["box", "als mich dem Schulhof irgendein"],
      ["spans", "start"],
    ],
  },
  {
    // Tim's start.mp4, HEVC in 10-bit colour: WebKit said it would decode
    // it and then failed on the first frame, "Decoder failure".
    name: "a picture the browser's decoder fails on comes from the Go side",
    steps: [["decoder fails"], ["from the app", 4]],
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
    // Tim made a build with make install, and its Updates page followed
    // pull request 143, picked by a build before it, and said it was not
    // checked yet with a Check that would never look by itself.
    name: "a build made on the Mac follows nothing until a channel is chosen, and then follows it",
    steps: [
      ["updates", { stored: "pr-143", list: "main,pr-143" }],
      ["channel", "Choose a channel"],
      ["choices", ["Branch main", "Pull request #143"], ""],
      ["status", "Built on this Mac", "It stays as it is until you choose a channel to follow."],
      ["no check"],
      ["follow", "Branch main"],
      ["at once", "Looking for a newer build"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
      ["channel", "Branch main"],
      ["choices", ["Branch main", "Pull request #143"], "Branch main"],
    ],
  },
  {
    name: "a build from a channel lists its channels and no Nothing",
    steps: [
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready"],
      ["choices", ["Branch main", "Pull request #20"], "Branch main"],
    ],
  },
  {
    name: "a pull request followed that closed says closed in the list and in the line under it",
    steps: [
      ["updates", { from: "main", stored: "pr-143", list: "main" }],
      ["status", "Pull request #143 is closed", "Nothing downloads until you choose what to follow next."],
      ["channel", "Pull request #143, closed"],
      ["choices", ["Branch main", "Pull request #143, closed"], "Pull request #143, closed"],
    ],
  },
];

// The Updates page: what its list says, and the line under the build. It
// is read in the page, so it can be read in the frame after a pick, see
// "follow".
function readUpdates() {
  const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
  const line = document.querySelectorAll(".card.asks > .item")[1];
  return {
    channel: text(document.querySelector("#channel .said > span:not(.room)")),
    head: text(line?.querySelector(".head")),
    more: text(line?.querySelector(".words .muted")),
    button: text(line?.querySelector("button")),
  };
}
const updatesPage = (page) => page.evaluate(readUpdates);

// Waits up to ten seconds for the Updates page to come to what ok says,
// and hands back what it said last.
async function updatesComeTo(page, ok) {
  let now = await updatesPage(page);
  for (let i = 0; i < 100 && !ok(now); i++) {
    await page.waitForTimeout(100);
    now = await updatesPage(page);
  }
  return now;
}

// The workspace's parts, where they stand and how big they are, as the
// stylesheet lays them out for the app's size.
const workspaceParts = (page) =>
  page.evaluate(() => {
    const box = (s) => document.querySelector(s)?.getBoundingClientRect();
    const parts = {
      viewer: box(".viewer"),
      picture: box(".screen"),
      "range picker": box(".track[aria-label='The range picker']"),
      "clip timeline": box(".track[aria-label='The clip timeline']"),
      settings: box(".stage > :first-child"),
      clips: box(".stage > :last-child"),
      "a settings field": box(".settings input"),
      "a clip card": box("aside ol li"),
    };
    const out = {};
    for (const [k, b] of Object.entries(parts)) if (b) out[k] = { x: b.left, y: b.top, w: b.width, h: b.height };
    const rows = [...document.querySelectorAll(".detail .row")].map((e) => e.getBoundingClientRect().bottom);
    out.app = { w: innerWidth, h: innerHeight, foot: Math.max(...rows) };
    return out;
  });

// What is wrong with the workspace as the app is dragged smaller a pixel at
// a time, by its height and then by its width, or null.
async function resizeProblem(page) {
  const sweeps = [
    // Down to the app's smallest, 960 by 640, see main.go.
    { by: "height", sizes: Array.from({ length: 361 }, (_, i) => [1500, 1000 - i]) },
    { by: "width", sizes: Array.from({ length: 741 }, (_, i) => [1700 - i, 1000]) },
  ];
  const sideColumns = await page.evaluate(() => {
    const s = document.querySelector(".stage");
    return s ? getComputedStyle(s).gridTemplateColumns : "";
  });
  if (!sideColumns) return "there is no workspace to resize";
  for (const { by, sizes } of sweeps) {
    const seen = [];
    for (const [width, height] of sizes) {
      await page.setViewportSize({ width, height });
      const parts = await workspaceParts(page);
      // The two sides measured from the app's own edges, the rail and a
      // space in front of the settings and the edge after the clips, so
      // the picture stands in the middle of the app.
      const [left, right] = await page.evaluate(() => {
        const [l, , r] = getComputedStyle(document.querySelector(".stage")).gridTemplateColumns.split(" ").map(parseFloat);
        const token = (n) => parseFloat(getComputedStyle(document.documentElement).getPropertyValue(n));
        return [token("--rail") + token("--gap") + l, r + token("--edge")];
      });
      if (Math.abs(left - right) > 1) return `at ${width} by ${height} the sides beside the picture are ${left} and ${right} wide from the app's edges`;
      for (const [k, b] of Object.entries(parts)) {
        if (k === "app") continue;
        for (const [m, v] of Object.entries(b)) {
          if (Math.abs(v - Math.round(v)) > 0.01) return `at ${width} by ${height} the ${k}'s ${m} is ${v}, between two pixels`;
        }
      }
      const edge = parts.app.w - (parts.clips.x + parts.clips.w);
      const foot = parts.app.h - parts.app.foot;
      seen.push({ parts, left, edge, foot, width, height });
    }
    const first = seen[0];
    for (const step of seen) {
      if (step.edge !== first.edge) return `at ${step.width} by ${step.height} the clips end ${step.edge} pixels from the app's right, not ${first.edge}`;
      if (step.foot !== first.foot) return `at ${step.width} by ${step.height} the workspace ends ${step.foot} pixels from the app's foot, not ${first.foot}`;
      if (by === "height" && step.left !== first.left) return `the sides beside the picture went from ${first.left} to ${step.left} wide while only the height changed`;
    }
    for (const k of Object.keys(first.parts)) {
      if (k === "app") continue;
      for (const m of ["x", "y", "w", "h"]) {
        let up = 0;
        let down = 0;
        for (let i = 1; i < seen.length; i++) {
          const d = seen[i].parts[k][m] - seen[i - 1].parts[k][m];
          if (d > 0.01) up++;
          if (d < -0.01) down++;
        }
        if (up && down) return `dragging the app's ${by}, the ${k}'s ${m} moved ${down} times one way and ${up} times the other`;
      }
    }
  }
  await page.setViewportSize({ width: 1500, height: 1000 });
  return null;
}

// The models the settings name: what finds clips, here or in the cloud,
// and what hears.
async function modelsNamed(page) {
  const s = await ask(page, "GetSettings");
  return { planner: s.planner, llmModel: s.llmModel, apiModel: s.apiModel, asrModel: s.asrModel };
}

// Where the clip's shots meet: every place one piece runs straight into
// the next, with nothing cut out between them.
const switches = (pieces) =>
  pieces.slice(1).filter((p, i) => Math.abs(p.start - pieces[i].end) < 0.0005).map((p) => p.start);

const pieceCount = (n) => `${n} piece${n === 1 ? "" : "s"}`;

// Frames a second of the episode on screen.
const rate = async (page) => (await ask(page, "Source", await episodeOn(page))).fps;

// What is wrong with the short Render wrote, against the video preview, or
// null. The frame of the short that stands for a moment is the frame of
// the episode the video preview shows there: the last one to begin at or
// before it. A short is made at one rate, and the episode's frames, on a
// phone, at uneven times, so a frame of the short stands for the moment
// it starts at, on the grid of the rate from where the picture starts, see
// OnFrames and cutOf in engine/render.go, and the playhead is put where
// the preview shows the frame that holds that moment: after it, and
// before the episode's next frame begins. Which frame begins where is read
// from the episode with ffprobe. The clip timeline is pinched in for it,
// and a frame whose time is still too short to click on is passed over,
// but every piece is looked at where it starts and where it ends.
// The render takes a frame that begins up to a millisecond after a moment
// for that moment, frameHair, and so does the video preview, so the
// playhead goes after that too and ends a millisecond and a half before
// the next frame begins.
async function asPreviewed(page, rate, before) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const probe = (args) => execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0"]).toString().trim();
  const got = probe(["-select_streams", "v:0", "-show_entries", "stream=r_frame_rate", short]);
  if (got !== rate) return `the short has ${got} frames a second, not ${rate}`;
  const begins = probe(["-select_streams", "v:0", "-show_entries", "packet=pts_time", on.path])
    .split(/\s+/).map(Number).filter((t) => Number.isFinite(t)).sort((a, b) => a - b);
  const fileStart = Number(probe(["-show_entries", "format=start_time", on.path])) || 0;
  const v0 = Math.max(0, begins[0] - fileStart);
  const pieces = entry.segments;
  if (before !== undefined && !(pieces[0].start < before && pieces[0].start < v0)) {
    return `the clip starts at ${pieces[0].start.toFixed(3)}, not before the picture at ${v0.toFixed(3)} and ${before}`;
  }
  // The pieces as the render takes them, see OnFrames.
  const grid = (t) => v0 + Math.round((t - v0) * fps) / fps;
  const slots = [];
  pieces.forEach((p, i) => {
    let start = grid(p.start);
    if (start < 0) start += 1 / fps;
    const end = Math.max(grid(p.end), start + 1 / fps);
    const n = Math.round((end - start) * fps);
    for (let q = 0; q < n; q++) slots.push({ piece: i, q, n, at: start + q / fps });
  });
  const { frames } = shortFrames(short);
  if (frames.length !== slots.length) return `the short has ${frames.length} frames, its pieces last ${slots.length}`;
  const after = (t) => begins.find((b) => b > t + 0.001) ?? Infinity;
  // Up close, pinched in to a tenth, so a frame is some pixels wide and
  // the edges of the pieces, which a click takes hold of, cover less than
  // one.
  const track = (await handles(page)).track;
  await page.mouse.move(track.x + track.w / 2, high(track));
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -230);
  await page.keyboard.up("Control");
  await page.waitForTimeout(300);
  const problems = [];
  let looked = 0;
  let missed = 0;
  const seenAt = new Set();
  for (let j = 0; j < slots.length; j++) {
    const { q, n, at } = slots[j];
    if (!(q < 3 || q >= n - 3 || q % 5 === 0)) continue;
    // Where the frame that holds the moment is the one on screen: from the
    // moment until the episode's next frame begins.
    const from = at + 0.0015;
    const to = after(at) - 0.0015;
    if (to - from < 0.002) continue;
    // Moved along with two fingers until the moment is well inside.
    for (let tries = 0; tries < 6; tries++) {
      const track = (await handles(page)).track;
      const off = (await xOf(page, (from + to) / 2)) - (track.x + track.w / 2);
      if (Math.abs(off) < track.w / 4) break;
      await page.mouse.move(track.x + track.w / 2, high(track));
      await page.mouse.wheel(off, 0);
      await page.waitForTimeout(150);
    }
    const g = await handles(page);
    const x0 = await xOf(page, from);
    const x1 = await xOf(page, to);
    // Clear of the clip's edges and of the edges of its cuts, which a
    // click takes hold of instead.
    const grips = [g.start, g.end, ...g.cuts, ...g.cutEdges].map((r) => [r.x - 3, r.x + r.w + 3]);
    if (x1 - x0 < 0.2) continue;
    // A click lands where the clip timeline's own track says, which can be
    // a pixel or two from where xOf reckons, so it is put right from where
    // the playhead went, as a hand would.
    const target = (from + to) / 2;
    let x = (x0 + x1) / 2;
    let placed = NaN;
    for (let tries = 0; tries < 4; tries++) {
      if (grips.some(([a, b]) => x > a && x < b)) break;
      await page.mouse.click(x, high(g.track));
      await settle(page);
      placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
      if (placed >= from && placed <= to) break;
      x += ((target - placed) * (x1 - x0)) / Math.max(to - from, 1e-6);
    }
    if (!(placed >= from && placed <= to)) {
      if (!Number.isNaN(placed)) missed++;
      continue;
    }
    looked++;
    seenAt.add(`${slots[j].piece} ${q < 3 ? "first" : q >= n - 3 ? "last" : "inside"}`);
    await filmedOnScreen(page);
    // The frame the preview shows, once it has settled on it: the short's
    // frame as soon as it comes, else what it stays on.
    const came = await page
      .waitForFunction((want) => window.__filmed?.() === want, frames[j], { polling: 50, timeout: 3000 })
      .then(() => true, () => false);
    const seen = came ? frames[j] : await filmedOnScreen(page);
    if (!came || (await filmedOnScreen(page)) !== frames[j]) {
      problems.push(`frame ${j} of the short, for ${at.toFixed(4)} s, is frame ${frames[j]} of the episode, the video preview shows ${seen} at ${placed.toFixed(4)}`);
    }
  }
  // Every piece looked at where it starts and where it ends, so at both
  // ends of the clip and on both sides of every cut, and inside.
  const unseen = pieces.flatMap((_, i) => ["first", "last"].filter((w) => !seenAt.has(`${i} ${w}`)).map((w) => `the ${w} frames of piece ${i + 1}`));
  if (unseen.length) problems.push(`the playhead could not be put on ${unseen.join(", ")}`);
  if (looked < 20) problems.push(`only ${looked} frames could be looked at, the playhead missed ${missed}`);
  console.log(`      ${looked} frames of ${slots.length} looked at, the playhead missed ${missed} more`);
  return problems.length ? problems.slice(0, 8).join("\n") + (problems.length > 8 ? `\nand ${problems.length - 8} more` : "") : null;
}

// What is wrong with the frames the video preview draws on the clip's
// edges, against the short Render wrote, or null. The playhead is put on
// each edge the way a hand puts it there, a click on the clip's start, on
// where each cut ends and on the clip's end, and the frame the video
// preview draws, read off the canvas, is the frame the short starts that
// piece with, or the short's last at the clip's end. Bug 1 of
// docs/DESIGN-REVIEW.md. An edge is kept to the millisecond, and at 29.97
// fps that lies just before its frame's start for 1400 of every 3000
// frames, where the video preview drew the frame before the one the short
// starts on. And a search left its edges between frames. The clip's end
// has to be where the search put it, and at least one edge just before
// its frame's start, or the walk is not about the case.
async function edgesAsRendered(page) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps, videoStart = 0 } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const { frames } = shortFrames(short);
  const pieces = entry.segments;
  // The frame an edge means, the one whose start is nearest, and each
  // piece's frames from there, the way the render cuts them, see
  // nearestFrame and cutOf in engine/render.go.
  const nearest = (t) => Math.round((t - videoStart) * fps);
  const firsts = [];
  let held = 0;
  for (const p of pieces) {
    const first = nearest(p.start);
    firsts.push(held);
    held += Math.max(nearest(p.end), first + 1) - first;
  }
  if (frames.length !== held) return `the short has ${frames.length} frames, its pieces hold ${held}`;
  // The clip's start and where each cut ends. Two pieces that meet, at a
  // camera switch or where the picture starts, have no cut between them.
  const edges = [{ what: "the clip's start", label: "Clip start", at: pieces[0].start, want: frames[firsts[0]] }];
  for (let i = 1; i < pieces.length; i++) {
    if (pieces[i].start <= pieces[i - 1].end) continue;
    edges.push({ what: `where cut ${i} ends`, label: `Where cut ${i} ends`, at: pieces[i].start, want: frames[firsts[i]] });
  }
  edges.push({ what: "the clip's end", label: "Clip end", at: pieces[pieces.length - 1].end, want: frames[frames.length - 1] });
  // Kept just before the start of the frame it means, by the rounding to
  // the millisecond.
  const before = edges.filter((e) => {
    const begins = videoStart + nearest(e.at) / fps;
    return e.at < begins - 1e-9 && begins - e.at < 0.0006;
  });
  if (!before.length) {
    return `no edge of the clip, at ${edges.map((e) => e.at).join(", ")}, lies just before its frame's start, the case this is about`;
  }
  if (Math.abs(entry.found[1] - pieces[pieces.length - 1].end) > 1e-9) {
    return `the clip ends at ${pieces[pieces.length - 1].end}, not at ${entry.found[1]} where the search put it, the other case this is about`;
  }
  await filmedOnScreen(page);
  const problems = [];
  for (const e of edges) {
    // High on the track, where the captions' edges below are not.
    const grip = await page
      .locator(`.clip-timeline [aria-label="${e.label}"]`)
      .boundingBox({ timeout: 3000 })
      .catch(() => null);
    if (!grip) {
      problems.push(`there is no ${e.label} to click`);
      continue;
    }
    await page.mouse.click(grip.x + grip.width / 2, high((await handles(page)).track));
    await settle(page);
    const placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    if (Math.abs(placed - e.at) > 1e-6) {
      problems.push(`a click on ${e.what} put the playhead at ${placed.toFixed(4)}, not ${e.at.toFixed(4)}`);
      continue;
    }
    const came = await page
      .waitForFunction((want) => window.__filmed?.() === want, e.want, { polling: 50, timeout: 3000 })
      .then(() => true, () => false);
    if (!came) {
      problems.push(`on ${e.what} at ${e.at.toFixed(3)} s the video preview shows frame ${await filmedOnScreen(page)}, the short frame ${e.want}`);
    }
  }
  console.log(`      ${edges.length} edges looked at, ${before.length} kept just before their frame`);
  return problems.length ? problems.join("\n") : null;
}

// The shorts in a folder, by name, finished ones only.
const shortsIn = (dir) => readdirSync(dir).filter((n) => n.endsWith(".mp4") && !n.endsWith(".part.mp4")).sort();

// The pictures beside a short, <name>-1.jpg and on, in their order.
function picturesOf(short) {
  const stem = basename(short, ".mp4");
  const mine = new RegExp(`^${stem.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}-(\\d+)\\.jpg$`);
  return readdirSync(dirname(short))
    .map((n) => [n, mine.exec(n)])
    .filter(([, m]) => m)
    .sort((a, b) => Number(a[1][1]) - Number(b[1][1]))
    .map(([n]) => join(dirname(short), n));
}

// Where the clip's short is, as the clip says, or empty when it has none.
async function shortOf(page, at) {
  const clip = (await ask(page, "Clips", at.path)).find((c) => c.plan === at.plan && c.id === at.clip);
  return clip?.rendered ?? "";
}

// What is wrong with the short Render wrote for a clip, read back from
// disk, or null. Its pieces are cut the way the render cuts them, see
// cutOf in engine/render.go: the frames from the one a piece's start is on
// up to the one before its end, at least one. Which frame of the episode a
// frame of the short is comes from its bands, see shortFrames, and is
// compared with the frame it should be, so a frame too many, one lost or
// one from the wrong place shows. The episode's sound is a steady tone, so
// padded silence at a cut, or sound that runs on after its picture, shows.
// The engine's own test of this is TestARenderCutsOnWholeFrames.
function shortProblem(entry, short, fps, cutsWanted) {
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const pieces = entry.segments;
  const problems = [];
  // The case is the one it says: a clip with cuts whose edges, on the
  // frames of the episode where the render puts them, fall between
  // milliseconds, where rounding to one moved a piece by a frame. The
  // engine keeps an edge where a word starts or ends, which here is a
  // whole millisecond, so it is the frame it lands on that counts.
  const edges = pieces.slice(1).flatMap((p, i) => [pieces[i].end, p.start]).map((t) => Math.round(t * fps) / fps);
  if (pieces.length - 1 < cutsWanted) problems.push(`the clip has ${pieces.length - 1} cuts, not ${cutsWanted}`);
  const whole = edges.filter((t) => Math.abs(t * 1000 - Math.round(t * 1000)) < 0.01);
  if (whole.length) problems.push(`cuts on frames that start on a whole millisecond, ${whole.join(" ")}, not the case this is about`);

  // Seen: the frames of each piece in order, each once, and nothing else.
  const want = [];
  const cuts = [];
  for (const p of pieces) {
    const first = Math.round(p.start * fps);
    const end = Math.max(Math.round(p.end * fps), first + 1);
    for (let k = first; k < end; k++) want.push(k);
    cuts.push(want.length / fps);
  }
  cuts.pop();
  const { frames, modulo } = shortFrames(short);
  if (frames.length !== want.length) problems.push(`the short has ${frames.length} frames, its pieces hold ${want.length}`);
  let wrongFrames = 0;
  for (let i = 0; i < Math.min(frames.length, want.length); i++) {
    if (frames[i] !== want[i] % modulo && ++wrongFrames <= 5) {
      problems.push(`frame ${i} of the short is frame ${frames[i]} of the episode, not ${want[i]}`);
    }
  }
  if (wrongFrames > 5) problems.push(`and ${wrongFrames - 5} frames more are the wrong frame`);

  // Heard: as long as the picture, the tone right up to each cut, and
  // quiet there only for the fade out and in, 15 ms, Fade in
  // engine/render.go, and that quiet where the picture cuts.
  const rate = 48000;
  const samples = shortSound(short);
  const heard = samples.length / rate;
  const pictured = want.length / fps;
  if (Math.abs(heard - pictured) > 0.025) {
    problems.push(`the sound is ${heard.toFixed(3)} s long, the picture ${pictured.toFixed(3)} s`);
  }
  // The loudest sample in the 2.5 ms around each one, more than one swing
  // of the tone, so how loud the tone is there.
  const loud = (k) => {
    let most = 0;
    for (let j = Math.max(0, k - 60); j < Math.min(samples.length, k + 60); j++) most = Math.max(most, Math.abs(samples[j]));
    return most;
  };
  const steady = loud(Math.round(0.5 * rate));
  cuts.forEach((cut, n) => {
    const from = Math.round((cut - 0.06) * rate);
    const to = Math.round((cut + 0.06) * rate);
    if (to > samples.length) {
      problems.push(`the sound ends before cut ${n + 1} at ${cut.toFixed(3)} s`);
      return;
    }
    let first = -1;
    let last = -1;
    for (let k = from; k < to; k++) {
      if (loud(k) < 0.25 * steady) {
        if (first < 0) first = k;
        last = k;
      }
    }
    if (first < 0) {
      problems.push(`cut ${n + 1} at ${cut.toFixed(3)} s has no fade in its sound`);
      return;
    }
    const quiet = (last - first) / rate;
    const mid = (first + last) / 2 / rate;
    if (quiet > 0.015) problems.push(`the sound is quiet for ${(1000 * quiet).toFixed(1)} ms at cut ${n + 1}, more than its fade`);
    if (Math.abs(mid - cut) > 0.003) {
      problems.push(`the sound cuts at ${mid.toFixed(4)} s, the picture at ${cut.toFixed(4)} s, cut ${n + 1}`);
    }
  });
  if (!problems.length) return null;
  const said = pieces.map((p) => `${p.start}-${p.end}`).join(" ");
  return `${problems.join("\n")}\npieces ${said} at ${fps} fps, the short ${short}`;
}

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const only = process.argv[2] ?? "";
let failures = 0;

for (const seq of sequences.filter((q) => q.name.includes(only))) {
  const { browser, page, errors } = await open(url);
  const at = await chosen(page);
  const watch = new Watch(page, at, errors);
  await watch.start();
  const marks = {};
  const looks = {};
  const kept = {};
  // The models the settings name, at "settings", and the episode that was
  // on screen before them.
  let named = null;
  let episodeBefore = "";
  // The Updates page in the frame after a channel was picked.
  let picked = null;
  let wrong = null;
  const done = [];
  const word = (text) => page.locator(".captions .word").filter({ hasText: new RegExp(`^${text}$`) }).first();

  for (const [what, ...args] of seq.steps) {
    const arg = args.length > 1 ? args : args[0];
    if (wrong || watch.failed) break;
    done.push(arg === undefined ? what : `${what} ${JSON.stringify(arg)}`);
    const s = await shown(page);
    switch (what) {
      case "frame": {
        for (let i = 0; i < 80 && !(await word(arg).count()); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await page.waitForTimeout(100);
        }
        await word(arg).click();
        await page.keyboard.press("Escape");
        await watch.step("frame", s);
        break;
      }
      case "click":
        await word(arg).click();
        await watch.step("click", s);
        break;
      case "press":
        await page.keyboard.press(arg);
        await watch.step(arg === "Enter" && !s.some((w) => w.focused) ? "open" : "press", s);
        break;
      case "type":
        await page.keyboard.type(arg);
        await watch.step("type", s);
        break;
      case "undo":
      case "redo":
        await page.evaluate((m) => window.__menu(m), what);
        await watch.step(what, s);
        break;
      case "cut at": {
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.dblclick(from + arg * (g.end.x - from), high(g.track));
        await watch.step("cut", s);
        break;
      }
      case "join": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g.cuts[arg - 1]), high(g.track));
        await watch.step("join", s);
        break;
      }
      case "trim": {
        const g = await handles(page);
        const [edge, px] = arg;
        await drag(page, middle(g[edge]), high(g.track), px, false);
        await watch.step("trim", s);
        break;
      }
      case "reset": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g[arg]), high(g.track));
        await watch.step("reset", s);
        break;
      }
      case "cut switch": {
        const at = switches((await engineState(page, watch.at)).segments)[arg - 1];
        const g = await handles(page);
        await page.mouse.dblclick(await xOf(page, at), high(g.track));
        await watch.step("cut", s);
        break;
      }
      case "trim past": {
        const [edge, n] = arg;
        const at = switches((await engineState(page, watch.at)).segments)[n - 1];
        const g = await handles(page);
        const to = await xOf(page, edge === "end" ? at - 2 : at + 2);
        await drag(page, middle(g[edge]), high(g.track), to - middle(g[edge]), false);
        await watch.step("trim", s);
        break;
      }
      case "look": {
        const fps = await rate(page);
        looks[arg] = [];
        for (const p of (await engineState(page, watch.at)).segments) {
          const at = (p.start + p.end) / 2;
          if (!(await seekTo(page, at, fps))) {
            wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
            break;
          }
          looks[arg].push({ ...p, at, frame: await cropFrame(page) });
        }
        break;
      }
      case "pieces": {
        const n = await page.locator(".clip-timeline .piece").count();
        if (n !== arg) wrong = `the clip timeline draws ${pieceCount(n)}, not ${arg}`;
        break;
      }
      case "framed": {
        // What a person sees first: the crop frame on every shot, where
        // it stood and with the same picture inside it. Then the pieces.
        const then = looks[arg];
        const fps = await rate(page);
        for (const was of then) {
          if (!(await seekTo(page, was.at, fps))) {
            wrong = `the video preview never showed the frame at ${was.at.toFixed(2)}`;
            break;
          }
          const now = await cropFrame(page);
          if (!now) {
            wrong = `at ${was.at.toFixed(2)} the video preview has no crop frame`;
          } else if (Math.abs(now.left - was.frame.left) > 0.005) {
            wrong = `at ${was.at.toFixed(2)} the crop frame stands ${(100 * now.left).toFixed(1)}% in, not ${(100 * was.frame.left).toFixed(1)}% as at "${arg}", and shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)}`;
          } else if (!sameLook(now.look, was.frame.look)) {
            wrong = `at ${was.at.toFixed(2)} the crop frame shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)} as at "${arg}"`;
          }
          if (wrong) break;
        }
        const n = await page.locator(".clip-timeline .piece").count();
        if (!wrong && n !== then.length) {
          wrong = `the clip timeline draws ${pieceCount(n)}, not ${then.length} as at "${arg}"`;
        }
        break;
      }
      case "shots": {
        const pieces = (await engineState(page, watch.at)).segments;
        const short = readShort(await shortOf(page, watch.at));
        // Each frame by the moment of the episode in its middle, through
        // the pieces, and what the crop frame showed there at the look.
        let into = 0;
        const placed = pieces.map((p) => {
          const q = { ...p, from: into };
          into += p.end - p.start;
          return q;
        });
        let compared = 0;
        for (let i = 0; i < short.frames.length && !wrong; i++) {
          const t = (i + 0.5) / short.fps;
          const p = placed.find((q) => t < q.from + q.end - q.start) ?? placed[placed.length - 1];
          const at = p.start + t - p.from;
          const was = looks[arg].find((l) => at >= l.start && at < l.end);
          if (was) compared++;
          if (was && !sameLook(short.frames[i], was.frame.look)) {
            wrong = `frame ${i} of the short, ${at.toFixed(2)} in the episode, shows ${sayLook(short.frames[i])}, not ${sayLook(was.frame.look)} as the crop frame did at "${arg}"`;
          }
        }
        // A short with no frame to compare proves nothing.
        if (!wrong && !compared) wrong = `no frame of the short was compared, of ${short.frames.length}`;
        if (process.env.VERBOSE) {
          console.log(`      ${compared} frames of ${short.frames.length} compared, pieces ${placed.map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ")}`);
        }
        // Heard: where one piece runs straight into the next, no 10 ms
        // within a tenth of a second either side is quieter than 80% of
        // the tone half a second before.
        for (const p of placed.slice(1)) {
          const before = placed[placed.indexOf(p) - 1];
          if (wrong || p.start - before.end > 0.0005) continue;
          const steady = loudness(short, p.from - 0.5);
          let least = { l: Infinity, t: 0 };
          for (let t = p.from - 0.1; t < p.from + 0.1; t += 0.0025) {
            const l = loudness(short, t);
            if (l < least.l) least = { l, t };
          }
          if (least.l < 0.8 * steady) {
            wrong = `at ${least.t.toFixed(4)} s into the short, by ${p.start.toFixed(2)} in the episode where two shots meet, the sound is down to ${((100 * least.l) / steady).toFixed(0)}% of the tone`;
          }
        }
        break;
      }
      case "cuts": {
        const t = await timeline(page);
        if (t?.cuts.length !== arg) wrong = `the clip timeline shows ${t?.cuts.length ?? "no"} cuts, not ${arg}`;
        break;
      }
      case "model": {
        const how = { holds: "hang=1&fail=0", fails: "hang=0&fail=1", answers: "hang=0&fail=0" }[arg];
        await control(url, `/model?${how}`);
        break;
      }
      case "add": {
        const [seconds, rate, timing] = [arg].flat();
        await control(url, `/pick?seconds=${seconds}${rate ? `&rate=${rate}` : ""}${timing ? `&timing=${timing}` : ""}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      }
      case "add cameras": {
        const [seconds, at] = arg;
        await control(url, `/pick?seconds=${seconds}&switch=${at}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      }
      case "render": {
        const on = await chosen(page);
        await page.waitForFunction(
          (r) => {
            const b = document.querySelector(r);
            return b && !b.disabled && b.textContent.trim().startsWith("Render");
          },
          "button.primary.render",
          { timeout: 60000, polling: 200 },
        );
        const renders = async () => (await ask(page, "Jobs")).filter((j) => j.kind === "render" && j.episode === on.path);
        const before = (await renders()).length;
        await page.locator("button.primary.render").click();
        let job = null;
        for (let i = 0; i < 600 && !job; i++) {
          await page.waitForTimeout(200);
          job = (await renders()).slice(before).find((j) => j.state === "done" || j.state === "failed") ?? null;
        }
        if (job?.state !== "done") wrong = `Render ${job ? `failed: ${job.error}` : "wrote no short in two minutes"}`;
        await watch.step("render", s);
        break;
      }
      case "head":
        await pressHead(page);
        await watch.step("head", s);
        break;
      case "speech":
        await control(url, `/speech?ms=${arg}`);
        break;
      case "hold":
        await control(url, `/hold?call=${arg[0]}&ms=${arg[1]}`);
        break;
      case "new and cancel": {
        const seen = await page.evaluate(
          () =>
            new Promise((done) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              const head = () => pane.querySelector(".listhead button.new");
              const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
              const frames = [];
              const pressed = text(head());
              head().click();
              const tick = () => {
                const b = head();
                frames.push({ says: text(b), off: !!b?.disabled });
                if (text(b) === "Cancel" && !b.disabled) {
                  b.click();
                  requestAnimationFrame(() => done({ pressed, frames, after: text(head()) }));
                } else if (frames.length < 300) requestAnimationFrame(tick);
                else done({ pressed, frames, after: text(head()) });
              };
              requestAnimationFrame(tick);
            }),
        );
        const greyed = seen.frames.filter((f) => f.off).length;
        if (seen.pressed !== "New") wrong = `the head said ${seen.pressed}, not New`;
        else if (seen.frames.some((f) => f.says !== "Cancel"))
          wrong = `after New the head said ${[...new Set(seen.frames.map((f) => f.says))].join(", ")}, not Cancel at once`;
        else if (greyed)
          wrong = `the head said Cancel greyed out for ${greyed} of ${seen.frames.length} frames after New, so a press there did nothing`;
        else if (seen.after !== "Cancelling") wrong = `Cancel pressed, the head said ${seen.after}, not Cancelling`;
        console.log(`      Cancel pressed ${seen.frames.length} frames after New, ${greyed} of them greyed out`);
        await watch.step("head", s);
        break;
      }
      case "settings": {
        episodeBefore = (await chosen(page))?.path ?? episodeBefore;
        await fromSidebar(page, () => page.locator("aside").getByText("Settings", { exact: true }).first().click());
        await page.locator(".card.finding").waitFor();
        await settle(page);
        named = await modelsNamed(page);
        // Every card that shakes from here on, by its classes.
        await page.evaluate(() => {
          window.__shook = [];
          new MutationObserver((changes) => {
            for (const c of changes) if (c.target.classList?.contains("shaking")) window.__shook.push(c.target.className);
          }).observe(document.body, { attributes: true, attributeFilter: ["class"], subtree: true });
        });
        break;
      }
      case "episode":
        await fromSidebar(page, () => page.locator("aside li", { hasText: episodeBefore.split("/").pop() }).first().click());
        await page.locator(".listhead button.new").first().waitFor();
        await settle(page);
        break;
      case "episode watched": {
        // Every frame the new episode's queue draws, read back from the
        // canvas on every animation frame until well after it settled. The
        // canvas is the one the episode before drew on, so only frames
        // drawn once the new episode's frames are open count.
        await preview(page, 0.04);
        await page.evaluate(() => {
          const was = window.__appFrames;
          window.__drawn = [];
          window.__watching = true;
          const watch = () => {
            if (!window.__watching) return;
            if (window.__appFrames && window.__appFrames !== was) {
              const n = window.__pictured();
              if (n >= 0 && n !== window.__drawn.at(-1)) window.__drawn.push(n);
            }
            requestAnimationFrame(watch);
          };
          requestAnimationFrame(watch);
        });
        await fromSidebar(page, () => page.locator("aside li", { hasText: episodeBefore.split("/").pop() }).first().click());
        await page.locator(".listhead button.new").first().waitFor();
        await settle(page);
        await page.waitForTimeout(1000);
        await page.evaluate(() => (window.__watching = false));
        break;
      }
      case "pick model": {
        await page.evaluate(() => (window.__shook = []));
        await page.locator(".card.finding button.pick").first().click();
        await page.waitForTimeout(400);
        const other = page.locator('[role="option"]:not([data-selected])');
        if (await other.count()) {
          await other.last().click();
          await page.waitForTimeout(400);
        }
        await settle(page);
        break;
      }
      case "remove model": {
        await page.evaluate(() => (window.__shook = []));
        if (arg === "language") {
          const fold = page.locator(".card.finding button.disclose-row");
          if ((await fold.getAttribute("aria-expanded")) !== "true") await fold.click();
          await page.locator('.card.finding button.bin[aria-label^="Remove "]').first().click();
        } else {
          await page.locator('.speech button.bin[aria-label^="Remove "]').first().click();
        }
        await page.waitForTimeout(400);
        break;
      }
      case "refused": {
        await page.waitForTimeout(200);
        const seen = await page.evaluate(() => ({
          shook: window.__shook ?? [],
          box: !!document.querySelector("dialog[open]"),
          list: !!document.querySelector('[role="listbox"]'),
          line: document.querySelector(".card.finding .words .line")?.textContent.trim() ?? "",
        }));
        const now = await modelsNamed(page);
        if (!seen.shook.some((c) => c.split(" ").includes(arg))) wrong = `the ${arg} card did not shake, shaken: ${JSON.stringify(seen.shook)}`;
        else if (seen.box) wrong = "a box opened";
        else if (seen.list) wrong = "the list opened";
        else if (JSON.stringify(now) !== JSON.stringify(named)) wrong = `the settings name ${JSON.stringify(now)}, not ${JSON.stringify(named)}`;
        else if (arg === "finding" && seen.line !== "A search is finding clips with it now.") wrong = `the line under Find clips with says "${seen.line}"`;
        break;
      }
      case "asks": {
        const box = page.locator("dialog[open]");
        if (!(await box.count())) {
          wrong = "no box asked whether to remove";
          break;
        }
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
        break;
      }
      case "hearing": {
        const jobs = await ask(page, "Jobs");
        if (!jobs.some((j) => j.kind === "search" && j.state === "running" && j.step === "hearing")) {
          wrong = `no search hears now: ${JSON.stringify(jobs.map((j) => [j.kind, j.state, j.step]))}`;
        }
        break;
      }
      case "changed": {
        const now = await modelsNamed(page);
        if (JSON.stringify(now) === JSON.stringify(named)) wrong = `the settings still name ${JSON.stringify(now)}`;
        break;
      }
      case "decoder fails": {
        await page.addInitScript(() => {
          const Real = window.VideoDecoder;
          window.VideoDecoder = class extends Real {
            constructor(init) {
              super(init);
              this.failWith = init.error;
            }
            decode() {
              setTimeout(() => this.failWith(new DOMException("Decoder failure", "EncodingError")), 0);
            }
          };
          window.VideoDecoder.isConfigSupported = async (config) => ({ supported: true, config });
        });
        const name = (await episodeOn(page)).split("/").pop();
        await page.reload();
        await page.waitForTimeout(1200);
        await fromSidebar(page, () => page.locator("aside li", { hasText: name }).first().click());
        await page.locator("aside ol li[data-key] button.pick").first().click();
        await settle(page);
        break;
      }
      case "on frames": {
        // Where the picture starts, read from the file by ffprobe rather
        // than from the app, which is what is being checked: the picture's
        // first timestamp after the file's own start.
        const path = await episodeOn(page);
        const { fps } = await ask(page, "Source", path);
        const read = (args) => Number(execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0", path]).toString().trim());
        const videoStart = read(["-select_streams", "v:0", "-show_entries", "stream=start_time"]) - read(["-show_entries", "format=start_time"]);
        const { segments } = await engineState(page, watch.at);
        // Before the picture starts there is no frame to land on: a start
        // dragged there takes the picture from its first frame.
        const edges = segments
          .flatMap((p) => [p.start, p.end])
          .slice(0, -1)
          .filter((t) => t >= videoStart - 1e-6);
        // Edges are kept to the millisecond, a thirtieth of a frame here.
        const off = edges
          .map((t) => ({ t, by: Math.abs((t - videoStart) * fps - Math.round((t - videoStart) * fps)) }))
          .filter((e) => e.by > 0.001 * fps + 1e-6);
        if (off.length) {
          wrong = `the picture starts at ${videoStart.toFixed(4)} s, and ${off
            .map((e) => `${e.t.toFixed(3)} is ${e.by.toFixed(2)} of a frame from its frames`)
            .join(", ")}`;
        }
        break;
      }
      case "resize":
        wrong = await resizeProblem(page);
        break;
      case "from the app": {
        const fps = await rate(page);
        const { segments } = await engineState(page, watch.at);
        const first = segments[0].start;
        const last = segments[segments.length - 1].end;
        for (let i = 1; i <= arg && !wrong; i++) {
          const at = first + ((last - first) * i) / (arg + 1);
          if (!(await seekTo(page, at, fps))) wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
        }
        const streams = await page.evaluate(() => window.__appFrames?.stats.streams ?? 0);
        if (!wrong && !streams) wrong = "the picture did not come from the Go side";
        const trouble = await page.evaluate(() => document.querySelector(".screen")?.textContent.trim() ?? "");
        if (!wrong && /decod/i.test(trouble)) wrong = `the video preview says "${trouble}"`;
        break;
      }
      case "restart": {
        const path = await page.evaluate(() => [...window.__calls].reverse().find((c) => c.name === "Clips")?.args[0]);
        await control(url, "/reopen");
        await page.reload();
        await page.waitForTimeout(1200);
        await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
        await settle(page);
        break;
      }
      case "wait for": {
        const came = await page
          .waitForFunction(
            (word) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              return pane?.querySelector(".listhead button.new")?.textContent.replace(/\s+/g, " ").trim() === word;
            },
            arg,
            { timeout: 60000, polling: 200 },
          )
          .then(() => true, () => false);
        if (!came) wrong = `the clip list's head says ${(await clipList(page))?.head}, not ${arg}, after a minute`;
        // The clip on screen may be a new one now, the first of a video
        // just added, and the watch follows it.
        else await watch.step("wait", s);
        break;
      }
      case "row": {
        const list = await clipList(page);
        if (!list?.rows.some((r) => r.what === arg)) wrong = `no row says "${arg}": ${JSON.stringify(list?.rows)}`;
        break;
      }
      case "cards": {
        const list = await clipList(page);
        if (list?.cards.length !== arg) wrong = `the clip list holds ${list?.cards.length} clips, not ${arg}`;
        break;
      }
      case "mark":
        marks[arg] = await engineState(page, watch.at);
        break;
      case "opened on the clip": {
        // The video preview of an episode that opens on a clip draws the
        // clip's first frame and nothing before it: no frame of the
        // episode's start on the way to the clip.
        const on = await chosen(page);
        const clip = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const fps = await rate(page);
        const first = Math.floor((clip.start + 0.001) * fps);
        const drawn = await page.evaluate(() => window.__drawn ?? []);
        if (!drawn.length) wrong = "the video preview drew no frame of the episode it opened";
        else if (drawn.some((n) => n !== first)) {
          wrong = `the episode opened on its clip at frame ${first} and the video preview drew frames ${drawn.join(", ")} on the way`;
        }
        break;
      }
      case "box": {
        const box = s.map((w) => w.text).join(" ");
        if (box !== arg) wrong = `the caption box reads "${box}", not "${arg}"`;
        break;
      }
      case "open": {
        const o = s.find((w) => w.focused);
        if (o?.text !== arg) wrong = `"${o?.text ?? "nothing"}" is open, not "${arg}"`;
        break;
      }
      case "same": {
        const now = await engineState(page, watch.at);
        if (!same(now, marks[arg])) wrong = `the captions are not what they were at "${arg}"\n${describe(marks[arg], now)}`;
        break;
      }
      case "spans": {
        const spans = (caps) => caps.captions.map((c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`).join(" ");
        const now = await engineState(page, watch.at);
        if (spans(now) !== spans(marks[arg])) {
          wrong = `the captions do not appear and go when they did at "${arg}"\nthen ${spans(marks[arg])}\nnow  ${spans(now)}\n${describe(marks[arg], now)}`;
        }
        break;
      }
      case "rendered": {
        const on = await chosen(page);
        const short = await shortOf(page, on);
        // The episode's own folder in the folder for shorts, named after
        // its file, see EpisodeName in engine/run.go.
        const shorts = realpathSync((await ask(page, "GetSettings")).outputDir);
        const folder = join(shorts, basename(on.path, extname(on.path)));
        const render = page.locator("button.primary.render");
        const again = await page
          .waitForFunction((r) => document.querySelector(r)?.textContent.trim() === "Render again", "button.primary.render", {
            timeout: 10000,
            polling: 200,
          })
          .then(() => true, () => false);
        const served = short
          ? await page.evaluate(async (p) => (await fetch(`/media/short?path=${encodeURIComponent(p)}`)).status, short)
          : 0;
        if (!short) wrong = "the clip says it has no short";
        else if (dirname(short) !== folder) wrong = `the short is at ${short}, not in the folder for shorts, ${folder}`;
        else if (!existsSync(short)) wrong = `the clip says its short is at ${short}, and nothing is there`;
        else if (served !== 200) wrong = `the app does not serve the short at ${short}: ${served}`;
        else if (!again) wrong = `the Render button says ${(await render.textContent())?.trim()}, not Render again`;
        else if (!(await page.getByRole("button", { name: "Show in folder" }).count())) wrong = "Show in folder is not there";
        break;
      }
      case "thumbnail": {
        const on = await chosen(page);
        const count = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.thumbnails.length ?? 0;
        const before = await count();
        await page.keyboard.press("t");
        let now = before;
        for (let i = 0; i < 50 && now === before; i++) {
          await page.waitForTimeout(100);
          now = await count();
        }
        if (now !== before + 1) wrong = `T left the clip with ${now} thumbnails, not ${before + 1}`;
        await watch.step("press", s);
        break;
      }
      case "keep": {
        const on = await chosen(page);
        const short = await shortOf(page, on);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const file = short && existsSync(short) ? statSync(short) : null;
        kept[arg] = { ...on, name: entry?.slug, short, file };
        if (!file) {
          wrong = `the clip has no short to keep${short ? `, nothing is at ${short}` : ""}`;
          break;
        }
        kept[arg].shorts = shortsIn(dirname(short));
        kept[arg].pictures = picturesOf(short).map((p) => ({ path: p, file: statSync(p) }));
        break;
      }
      case "by its words": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        if (!short || !existsSync(short)) wrong = "the clip has no short in its folder";
        else if (basename(short) !== `${entry?.slug}.mp4`) wrong = `the short is called ${basename(short)}, not ${entry?.slug}.mp4`;
        break;
      }
      case "pictured": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        const want = entry?.thumbnails.length ?? 0;
        const got = short ? picturesOf(short) : [];
        const named = got.map((p) => basename(p)).join(", ");
        if (!short) wrong = "the clip says it has no short";
        else if (want === 0) wrong = "the clip has no thumbnails, so there is nothing to look for";
        else if (got.length !== want || got.some((p, i) => basename(p) !== `${basename(short, ".mp4")}-${i + 1}.jpg`)) {
          wrong = `beside ${short} lie ${named || "no pictures"}, not the clip's ${want}`;
        }
        break;
      }
      case "in place": {
        const was = kept[arg];
        const on = await chosen(page);
        const short = await shortOf(page, on);
        const now = short && existsSync(short) ? statSync(short) : null;
        const added = short ? shortsIn(dirname(short)).filter((n) => !was.shorts.includes(n)) : [];
        if (short !== was.short) wrong = `rendered again, the short is at ${short || "nowhere"}, not ${was.short}`;
        else if (!now || now.mtimeMs === was.file.mtimeMs) wrong = `rendered again, ${short} was not written again`;
        else if (added.length) wrong = `rendered again, ${added.join(", ")} appeared beside ${basename(short)}`;
        break;
      }
      case "add namesake":
        await control(url, `/pick?seconds=${arg}&namesake=1`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      case "words again":
        await control(url, "/speech?ms=0&from=0");
        break;
      case "updates": {
        const q = new URLSearchParams({ from: arg.from ?? "", stored: arg.stored ?? "", list: arg.list ?? "" });
        await control(url, `/updates?${q}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Updates", { exact: true }).first().click());
        await page.locator("#channel").waitFor();
        await settle(page);
        break;
      }
      case "follow": {
        await page.locator("#channel").click();
        await page.getByRole("option", { name: arg, exact: true }).click();
        const read = await page.evaluate(
          (src) => new Promise((done) => requestAnimationFrame(() => done(new Function(`return (${src})()`)()))),
          readUpdates.toString(),
        );
        picked = { name: arg, ...read };
        break;
      }
      case "channel": {
        const now = await updatesComeTo(page, (u) => u.channel === arg);
        if (now.channel !== arg) wrong = `the list says ${now.channel || "nothing"}, not ${arg}`;
        break;
      }
      case "choices": {
        const [names, want] = arg;
        await page.locator("#channel").click();
        await page.getByRole("option").first().waitFor();
        const rows = await page.evaluate(() =>
          [...document.querySelectorAll('[role="option"]')].map((o) => ({
            name: (o.textContent ?? "").replace(/\s+/g, " ").trim(),
            picked: o.hasAttribute("data-selected") || o.getAttribute("aria-selected") === "true",
          })),
        );
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
        const got = rows.map((r) => r.name);
        const on = rows.filter((r) => r.picked).map((r) => r.name);
        if (JSON.stringify(got) !== JSON.stringify(names)) wrong = `the list offers ${JSON.stringify(got)}, not ${JSON.stringify(names)}`;
        else if (JSON.stringify(on) !== JSON.stringify(want ? [want] : [])) {
          wrong = `the list has ${on.length ? on.join(", ") : "nothing"} picked, not ${want || "nothing"}`;
        }
        break;
      }
      case "status": {
        const [head, more] = [arg].flat();
        const now = await updatesComeTo(page, (u) => u.head === head);
        if (now.head !== head) wrong = `the line under the build says ${now.head || "nothing"}, not ${head}`;
        else if (more && !now.more.includes(more)) wrong = `under ${head} it says ${JSON.stringify(now.more)}, not ${JSON.stringify(more)}`;
        break;
      }
      case "no check": {
        const now = await updatesPage(page);
        if (now.button) wrong = `the line under the build offers ${now.button}`;
        break;
      }
      case "at once":
        if (picked?.channel !== picked?.name) wrong = `in the frame after picking ${picked?.name}, the list said ${picked?.channel || "nothing"}`;
        else if (picked.head !== arg) wrong = `in the frame after the pick, the line under the build said ${picked.head || "nothing"}, not ${arg}`;
        break;
      case "apart": {
        const [label, number] = [arg].flat();
        const was = kept[label];
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        const now = existsSync(was.short) ? statSync(was.short) : null;
        const theirs = await shortOf(page, was);
        if (on.path === was.path) wrong = `the clip on screen is of the episode kept at "${label}"`;
        else if (entry?.slug !== was.name) wrong = `the clips are named ${was.name} and ${entry?.slug}, which never meet in one folder`;
        else if (!short) wrong = "the clip on screen says it has no short";
        else if (short === was.short) wrong = `both episodes' shorts are ${short}`;
        else if (!now || now.ino !== was.file.ino || now.mtimeMs !== was.file.mtimeMs) {
          wrong = `the second render wrote over the first episode's short at ${was.short}`;
        } else if (theirs !== was.short) {
          wrong = `the first episode's clip says its short is ${theirs || "nowhere"}, not ${was.short}`;
        } else if (number && basename(short) !== `${was.name}${number}.mp4`) {
          wrong = `the second short is called ${basename(short)}, not ${was.name}${number}.mp4`;
        } else {
          const gone = was.pictures.filter((p) => {
            const now = existsSync(p.path) ? statSync(p.path) : null;
            return !now || now.ino !== p.file.ino || now.mtimeMs !== p.file.mtimeMs;
          });
          if (gone.length) wrong = `the second render took or wrote over the first episode's pictures ${gone.map((p) => basename(p.path)).join(", ")}`;
        }
        break;
      }
      case "as previewed": {
        const [rate, before] = [arg].flat();
        wrong = await asPreviewed(page, rate, before);
        break;
      }
      case "edges as rendered":
        wrong = await edgesAsRendered(page);
        break;
      case "short": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const { fps } = await ask(page, "Source", on.path);
        const short = await shortOf(page, on);
        wrong = shortProblem(entry, short, fps, arg);
        break;
      }
      default:
        throw new Error(`no step called ${what}`);
    }
  }
  const failed = wrong ?? (watch.failed && `broke: ${watch.failed.rule}\n${watch.failed.detail}`);
  console.log(`${failed ? "FAIL" : "ok  "}  ${seq.name}`);
  if (failed) {
    failures++;
    console.log(`      after ${done.join(", ")}\n      ${failed.replaceAll("\n", "\n      ")}`);
  }
  await browser.close();
}
process.exit(failures ? 1 : 0);
