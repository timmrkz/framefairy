// The sequences about Report a Problem, the toasts and dictation. They are
// cases found by hand, kept so they stay fixed. Their steps are said in
// steps.mjs beside this file, and in common/ for the steps of every folder.
// See ../sequences.mjs.
export default [
  {
    // Plan row 2.189. Tim asked to talk into Report a Problem rather than
    // type: what he says appears while he says it, where the cursor was,
    // and Cmd+Z takes it back like anything typed, all of it at once. The
    // microphone is the bridge's, which plays a hum with a pause in it as
    // fast as it is said, and the speech model its stand-in. What came
    // before the pause is settled while it listens, the rest when it
    // stops.
    name: "Dictate writes what is said while it is said, where the cursor was, and undo takes it back",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["microphone moves"],
      ["dictate", "button"],
      ["report reads", "It * froze."],
      ["undo"],
      ["report reads", "It froze."],
    ],
  },
  {
    // Plan row 2.189. Tim undid what he typed while he talked, and Cmd+Y
    // did not bring it back: the words that arrived meanwhile were steps
    // of their own, which emptied what there was to redo. They are not
    // steps any more, so undo and redo while talking are about what was
    // typed alone.
    name: "While dictating, undo and redo are about what was typed",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["type", "X"],
      ["undo"],
      ["dictate", "settled"],
      ["redo"],
      // X was typed at the end of what was said, so what is said next
      // comes after it, with a space on either side.
      ["dictate", "hearing"],
      ["report reads", "It * X * froze."],
      ["dictate", "enter"],
      ["report reads", "It * X * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim asked for dictation the way claude.ai does it:
    // Enter keeps what was said, as the microphone does. Escape removed
    // it then, and Tim lost a long report to an Escape meant for the app,
    // with nothing Cmd+Z could bring back, issue #355. Escape keeps it
    // too now, as it does on a Mac, and Cmd+Z takes it back as one step
    // and Cmd+Shift+Z brings it back again.
    name: "Enter and Escape keep what was dictated, and undo takes it back",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "enter"],
      ["report reads", "It * froze."],
      ["dictate", "escape"],
      ["report grew"],
      ["undo"],
      ["report as before"],
      ["redo"],
      ["report as before", "grown"],
    ],
  },
  {
    // Plan row 2.189. Tim could not click into the words still being
    // heard, nor at the end of their line, to put the cursor there or to
    // correct a word while he talked. A click in them settles them where
    // they are, and what is heard after comes after them.
    name: "A click in the words being heard puts the cursor there while dictating",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["click heard"],
      ["type", "X"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "It *X* * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim typed a few words after what he had said and
    // talked on, and what he said next went in front of what he typed,
    // pushing it along. What is typed at the end of the words being said
    // goes after them, and what is said next goes after what was typed.
    name: "What is said after typing at the end of a dictation comes after the typing",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["dictate", "settled"],
      ["type", "Y"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "It * Y * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim typed at the end of what he said, moved the
    // cursor with the arrow keys and typed there, and talked on. On his
    // Mac the letters came out backwards, and Enter lost everything said
    // before he moved. What was said stays one piece wherever the person
    // types, and Enter keeps all of it, in order.
    name: "Typing elsewhere while dictating keeps every word, in order",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["dictate", "settled"],
      ["type", "Y"],
      // The start of the report, not of the line: what is said after Y
      // pushes the cursor along, onto a line further down.
      ["press", "Control+Home"],
      ["press", "ArrowRight"],
      ["press", "ArrowRight"],
      ["type", "Z"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "ItZ * Y * froze."],
    ],
  },
  {
    // Issue #373. Tim ended a dictation and pressed Enter at once for a
    // new line, while what he said was still being written down. The
    // words were written in and the cursor put back in front of his new
    // line, so what he typed next went before it. The cursor stays where
    // he put it.
    name: "A new line typed while what was said is written down stays where it was typed",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "escape, then enter"],
      ["type", "Next"],
      ["report reads", "It froze. *\nNext"],
    ],
  },
  {
    // Issue #373. Shift+Enter while dictating put the new line after the
    // words, and what was said next still went on the line above the
    // cursor. The new line is typed at the end of what was said, like a
    // letter, and what is said next goes on the new line.
    name: "Shift+Enter while dictating starts a new line for what is said next",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["press", "Shift+Enter"],
      ["caret at words"],
      ["dictate", "hearing"],
      ["caret at words"],
      ["dictate", "escape"],
      ["type", "Next"],
      ["report reads", "It froze. *\n*Next"],
    ],
  },
  {
    // Issue #373. Tim pressed Shift+Enter twice while dictating on his
    // Mac. WebKit put the second new line after the words being heard, so
    // they went on the line above the cursor, and once kept, the new lines
    // were blocks of their own, which showed as an empty line at the end.
    // Every new line is text, in the order it was typed.
    name: "Two Shift+Enters while dictating make two new lines, kept as text",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["press twice", "Shift+Enter"],
      ["caret at words"],
      ["dictate", "hearing"],
      ["caret at words"],
      ["dictate", "escape"],
      ["report is text"],
      ["type", "Next"],
      ["report reads", "It froze. *\n\n*Next"],
    ],
  },
  {
    // Issue #573. Tim said a lot quickly and pressed Shift+Enter, and the
    // words the speech model wrote down after went below the new line,
    // though he said them before it. What is said before the new line
    // goes above it, however far behind the speech model is.
    name: "What is said before Shift+Enter goes above the new line, however far behind the speech model is",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["talk ahead", 10],
      ["press", "Shift+Enter"],
      ["dictate", "hearing"],
      ["dictate", "escape"],
      ["said before", "\n"],
    ],
  },
  {
    // Issue #584. The same for words typed at the end of what was said:
    // what was said before them goes in front of them, however far
    // behind the speech model is, and what is said after them after.
    // Typed on a new line before anything said there was shown, they
    // keep a space from the words that come in front of them.
    name: "What is said before typing at the end of a dictation goes in front of the typing, however far behind the speech model is",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["talk ahead", 10],
      ["press", "Shift+Enter"],
      ["talk ahead", 10],
      ["type", "XYZ"],
      ["dictate", "hearing"],
      ["dictate", "escape"],
      ["said before", "XYZ"],
    ],
  },
  {
    // Issue #584. The words still to be written down where a letter was
    // typed cannot be typed in, and Backspace took them all away at once,
    // with the words the speech model had not yet written down. A key
    // waits for them instead.
    name: "Backspace after typing at the end of a dictation takes away no words still to be written down",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["talk ahead", 10],
      ["type", "X"],
      ["press", "Backspace"],
      ["press", "Backspace"],
      ["press", "Backspace"],
      ["words kept"],
      ["dictate", "escape"],
      ["words kept"],
    ],
  },
  {
    // Issue #573. Tim pressed Shift+Enter while dictating until the new
    // lines went past the foot of the field, and the field stayed where
    // it was while the cursor went on down out of view. Only the next
    // words he said brought it back. The arrow keys did the same. The
    // field keeps the cursor in view, the way every text field does,
    // while dictating and after.
    name: "Shift+Enter and the arrow keys keep the cursor in view in the report",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve", "end"],
      ["dictate", "start"],
      ["press, the cursor in view", ["Shift+Enter", 12]],
      // At the end of the report, down to its padding, where the words
      // said next leave it too. Tim saw the field scroll a few pixels
      // with the next words after a new line.
      ["report at end"],
      ["press, the cursor in view", ["ArrowUp", 24]],
      ["press, the cursor in view", ["ArrowDown", 24]],
      ["dictate", "escape"],
      ["press, the cursor in view", ["ArrowUp", 24]],
      ["press, the cursor in view", ["Shift+Enter", 12]],
    ],
  },
  {
    // Tim, 29 October. While dictating, the cursor after the words was one
    // the field drew itself, unlike the one it has the rest of the time,
    // since it sat after a part of the field that cannot be typed in,
    // where WebKit draws none. ArrowDown there found no line to go to and
    // scrolled the field instead, which bounced, and ArrowUp went from the
    // wrong place, to the top of the report at times. The cursor is the
    // field's own, in text the browser moves it from, and the keys move
    // nothing that is not there to move, after a new line too.
    name: "While dictating, the cursor after the words is the field's own, and ArrowDown there moves nothing",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve", "end"],
      ["dictate", "start"],
      ["dictate", "hearing"],
      ["caret at words"],
      ["report at end"],
      ["press at the end", ["ArrowDown", 3]],
      ["press", "Shift+Enter"],
      ["caret at words"],
      ["report at end"],
      ["press at the end", ["ArrowDown", 3]],
      ["dictate", "hearing"],
      ["caret at words"],
      ["dictate", "enter"],
    ],
  },
  {
    // Tim, 29 October, 20.55. Moving up and down the report with the arrow
    // keys, the field jumped several lines at a time, and at the last
    // line went a few pixels past it and back. It scrolls a line at a
    // time, while dictating and after, and never past its end.
    name: "The arrow keys scroll the report a line at a time, while dictating and after",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen\nFifteen\nSixteen\nSeventeen\nEighteen\nNineteen\nTwenty", "end"],
      ["press, a line at a time", ["ArrowUp", 22]],
      ["press, a line at a time", ["ArrowDown", 22]],
      ["report at end"],
      ["dictate", "start"],
      ["press, a line at a time", ["ArrowUp", 24]],
      ["press, a line at a time", ["ArrowDown", 24]],
      ["report at end"],
      ["press at the end", ["ArrowDown", 3]],
      ["dictate", "enter"],
    ],
  },
  {
    // Tim, 29 October. Tim moved the cursor up with the arrow keys while
    // he talked, and the field scrolled up with the cursor and back down
    // to the words by turns, and the cursor went out of view. It is the
    // cursor the field keeps in view: the words coming in move it only
    // while the cursor is after them.
    name: "Moving the cursor up while dictating keeps it in view, and the words coming in leave the field where it is",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen\nFifteen\nSixteen", "end"],
      ["dictate", "start"],
      ["report follows"],
      ["press, the cursor in view", ["ArrowUp", 14]],
      ["field kept"],
      ["report stays"],
      ["cursor in view"],
      ["dictate", "enter"],
    ],
  },
  {
    // Issue #373. Tim ended a dictation on his Mac and the cursor jumped
    // while the last words were written down, to the front of the line or
    // past where the words would end, and back once they came. It stays
    // drawn after the words until they are in.
    name: "The cursor stays after the words while the last of them are written down",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["caret at words"],
      // The last words take a while to write down, the way they do on a
      // Mac after a long sentence: the step holds them back while it looks.
      ["dictate", "escape, the cursor stays"],
    ],
  },
  {
    // Issue #373. Tim felt the cursor was not always pushed along by
    // what he said. After typing at the end of what was said, it stayed
    // behind and the words heard next ran on to the right of it. It is
    // pushed along again as soon as more is heard.
    name: "After typing at the end of a dictation, what is said next pushes the cursor along",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "start"],
      ["dictate", "settled"],
      ["type", "Y"],
      ["dictate", "hearing"],
      ["caret at words"],
      ["dictate", "enter"],
    ],
  },
  {
    // Issue #404. Tim ends a dictation with Enter and makes the report
    // with Tab and Enter. Tab from the field went to the first picture,
    // since macOS tabs past buttons, or with none to nothing he could
    // see. It goes to Report, then through the pictures and back.
    name: "Tab from the report's field goes to Report, with pictures and without",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["tab", "Report"],
      ["tab", "the field"],
      ["shift tab", "Report"],
      ["shift tab", "the field"],
      ["attach", 2],
      ["write", " Again.", "end"],
      ["tab", "Report"],
      ["tab", "picture 1"],
      ["tab", "picture 2"],
      ["tab", "the field"],
      ["shift tab", "picture 2"],
    ],
  },
  {
    // Tim ended a dictation with Enter and pressed Tab while the last
    // words were still being written down. Tab put the focus on Report,
    // and when the words came the field took it back, so Tab looked as if
    // it passed Report over. The words are held back until after the Tab.
    name: "Tab right after a dictation leaves the focus on Report once the words are written",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["dictate", "enter, then tab"],
      ["report reads", "It froze. *"],
    ],
  },
  {
    // Issue #404. Tab while dictating stopped it, and Tim wants only Enter,
    // Escape and the button to end a dictation. Tab does nothing then. A
    // picture is attached so Tab has somewhere else it could go.
    name: "Tab while dictating does nothing",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["attach", 1],
      ["dictate", "start"],
      ["tab", "the field"],
      ["dictating"],
      ["dictate", "enter"],
      ["report reads", "It froze. *"],
    ],
  },
  {
    // Issue #415. On an empty report Tab still went to Report, lit, with
    // nothing to send. Report and Clear are off until there is something.
    name: "Report and Clear are off with nothing to report",
    steps: [
      ["bugs"],
      ["report on", false],
      ["tab", "the field"],
      ["write", "x", "end"],
      ["report on", true],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["report on", false],
      ["attach", 1],
      ["report on", true],
    ],
  },
  {
    // Issue #415. Tim asked for one button that empties a report that
    // turned out to be for nothing, after asking, rather than removing
    // the text and every picture by hand. Then that Cmd+Z brings it all
    // back and Cmd+Y clears it again.
    name: "Clear asks first, empties the words and the pictures, and undo brings them back",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["attach", 2],
      ["clear", "cancel"],
      ["pictures", 2],
      ["report reads", "It froze."],
      ["clear", "clear"],
      ["pictures", 0],
      ["report on", false],
      ["undo"],
      ["report reads", "It froze."],
      ["pictures", 2],
      ["redo"],
      ["report reads", ""],
      ["pictures", 0],
      ["undo"],
      ["pictures", 2],
      ["close report"],
      ["bugs"],
      ["report reads", "It froze."],
      ["pictures", 2],
      ["clear", "clear"],
      ["close report"],
      ["bugs"],
      ["report reads", ""],
      ["pictures", 0],
    ],
  },
  {
    // Issue #554. Undo of Clear puts the pictures back one call at a
    // time. A Redo pressed before they were answered cleared between
    // them, and the second picture came back after the clear, with the
    // words gone. The first put back is held at the Go side until Redo
    // has been pressed, the way a busy machine delivers it late.
    name: "Redo pressed while Undo of Clear is putting the pictures back clears them all",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["attach", 2],
      ["clear", "clear"],
      ["pictures", 0],
      ["hold", "PutBackPicture"],
      ["undo then redo"],
      ["report reads", ""],
      ["pictures", 0],
      ["close report"],
      ["bugs"],
      ["report reads", ""],
      ["pictures", 0],
    ],
  },
  {
    // Issue #415. Clear's undo lasts only until something else is typed,
    // attached or removed. Typing after Clear starts a new report, and
    // Cmd+Z then takes back the typing, never the pictures Clear removed.
    name: "Typing after Clear ends what Cmd+Z brings back",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["attach", 2],
      ["clear", "clear"],
      ["write", "x", "end"],
      ["undo"],
      ["pictures", 0],
      ["report reads", ""],
    ],
  },
  {
    // Issue #404. Tab and Enter straight after Enter, while the last words
    // are still written down, make the report with every word said in it.
    // The stop is held on its way to the Go side, so the last words only
    // come once Tab and Enter are pressed.
    name: "Tab and Enter right after a dictation make the report with every word said",
    steps: [
      ["bugs"],
      ["write", "It froze.", "end"],
      ["attach", 1],
      ["dictate", "enter, then tab and enter"],
      ["report made", 1, "It froze. *"],
    ],
  },
  {
    // Plan row 2.244. Tim asked for one report window at a time: Bugs
    // opens it, Bugs again brings it forward, and once it is closed Bugs
    // opens a new one. Bugs is lit while it is open.
    name: "Bugs opens one report window and brings it back",
    steps: [
      ["windows", 0, 0],
      ["bugs"],
      ["windows", 1, 0],
      ["bugs"],
      ["windows", 1, 1],
      ["close report"],
      ["windows", 1, 1],
      ["bugs"],
      ["windows", 2, 1],
    ],
  },
  {
    // Plan row 2.244. Closing the report window loses nothing: what was
    // written and the pictures are there when Bugs opens it again.
    name: "What is written and attached stays when the report window closes",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["attach", 1],
      ["pictures", 1],
      ["close report"],
      ["bugs"],
      ["report reads", "It froze."],
      ["pictures", 1],
    ],
  },
  {
    // Plan row 2.244. Pictures come from the paperclip's box, a drop on
    // the window and a paste into the field, and a file that is not a
    // picture is left out with the reason. Everything lines up.
    name: "Pictures come from the paperclip, a drop and a paste, and what is not one is left out",
    steps: [
      ["bugs"],
      // Held, so the toast's ten seconds cannot run out on a busy machine
      // before the good drop after it is read.
      ["hold the clock"],
      ["attach", 1],
      ["drop pictures", 2, "notes.txt"],
      ["toast", "notes.txt cannot be attached because it is not a picture or a screen recording"],
      // A good drop after it says nothing of its own and leaves the old
      // reason to run its time, issue #488. The reason is still true, and
      // a toast goes by its own time, the way each reason Tim dropped one
      // after another stays in the stack. It once was a line beside the
      // buttons, which the review of #217 asked a good drop to clear.
      ["drop pictures", 1],
      ["toast", ["notes.txt cannot be attached because it is not a picture or a screen recording"]],
      ["paste picture"],
      ["pictures", 5],
      ["report lines up"],
    ],
  },
  {
    // Issue #362. Tim saw the buttons a size of their own, mostly much
    // wider than their words, and some of them growing past it on his
    // Mac. A button is as wide as the words it shows, whatever they are.
    name: "every button is as wide as its words and no wider",
    steps: [
      ["buttons fit", ["Search", "Render"]],
      ["settings"],
      ["buttons fit", ["Choose…", "Check again"]],
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready"],
      ["buttons fit", ["Relaunch"]],
      ["bugs"],
      ["buttons fit", ["Clear", "Report"]],
    ],
  },
  {
    // Issue #326. Tim attached a screen recording made on his Mac and
    // it was turned away as not a PNG or JPEG picture. A recording is
    // attached beside the pictures, shows a still of its first frame and
    // its length, and goes in the report as a recording.
    name: "A screen recording is attached, shown by its first frame and its length, and goes in the report",
    steps: [
      ["bugs"],
      ["drop pictures", 1],
      ["drop recording"],
      ["toast", ""],
      ["pictures", 2],
      ["recording tile", "0:03"],
      ["report lines up"],
      ["report", [1, 1]],
    ],
  },
  {
    // Plan row 2.244. Tim attached screenshots of every shape: tall ones
    // showed only a strip of their middle, and a small tall one showed
    // nothing at all. Every picture is drawn whole, in the middle of its
    // tile and as large as it lets it be.
    name: "Pictures of every shape are drawn whole in their tiles",
    steps: [
      ["bugs"],
      ["drop pictures", "446x738 152x338 1492x190 142x104 786x504 380x1766"],
      ["pictures", 6],
      ["pictures fit"],
    ],
  },
  {
    // Plan row 2.244. Tim ended a dictation at the end of a long report
    // with Enter and with the microphone, and the microphone left the
    // field scrolled half a line up where Enter kept its end in view.
    // Both keep the end of what was said in view.
    name: "Stop and Enter both leave the end of a long report in view",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen", "end"],
      ["dictate", "button"],
      ["report at end"],
      ["dictate", "enter"],
      ["report at end"],
    ],
  },
  {
    // Plan row 2.244. Tim scrolled up to read while he talked, and the
    // field pulled him back down with every word. It follows the words
    // the way a terminal follows its output, until scrolled away from
    // them and again once back at them. And the cursor stood at the front
    // of the last line while the words ran on.
    name: "While dictating the field follows the words until scrolled away, like a terminal",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen\nFifteen\nSixteen", "end"],
      ["dictate", "start"],
      ["report follows"],
      ["caret at words"],
      ["report scroll", -120],
      // A key that neither moves the cursor nor types leaves the field
      // where it was scrolled to, issue #573.
      ["press", "Shift"],
      ["report stays"],
      ["report scroll", 5000],
      ["report follows"],
      ["caret elsewhere"],
      ["dictate", "button"],
    ],
  },
  {
    // Plan row 2.244. A report takes ten pictures, and the eleventh is
    // left out with the reason.
    name: "A report takes ten pictures and says why it leaves out the eleventh",
    steps: [
      ["bugs"],
      ["drop pictures", 10],
      ["pictures", 10],
      ["attach", 1],
      ["toast", "would be more than the 10 pictures and recordings a report takes"],
      ["pictures", 10],
    ],
  },
  {
    // Plan row 2.244. The bin on a picture removes it, the picture keeps
    // its place and offers itself back, and once its time runs out the
    // grid closes over it.
    name: "A removed picture can be put back, and then the grid closes over it",
    steps: [
      ["bugs"],
      ["attach", 3],
      ["remove picture", 2],
      ["put back picture", 2],
      ["remove picture", [2, "tile"]],
      ["put back picture", 2],
      ["pictures", 3],
      ["remove picture", [2, "key"]],
      ["hold the clock"],
      ["removed goes"],
      ["pictures", 2],
    ],
  },
  {
    // Plan row 2.244. Tim clicked Bugs, and the sidebar stayed open behind
    // the report window until he clicked back into the app and moved the
    // pointer.
    name: "Bugs leaves the sidebar closed once the report window has the focus",
    steps: [["bugs, the pointer gone"]],
  },
  {
    // Plan row 2.244. Report makes one zip with what was written and the
    // pictures, and the next report starts empty.
    name: "Report makes one zip with the pictures, and the next report starts empty",
    steps: [
      ["bugs"],
      ["write", "It froze.", 0],
      ["attach", 2],
      ["report lines up"],
      ["report", 2],
      ["close report"],
      ["bugs"],
      ["pictures", 0],
    ],
  },
  {
    // Plan row 2.189. Tim talked past the longest a recording can be and
    // lost what he said after it, with nothing to say why. A recording
    // nobody talks into stops by itself, keeps every word as Stop would,
    // and says why under the field. The bridge says the quiet is over
    // once the person has fallen silent, rather than after two minutes.
    name: "Dictation nobody talks into stops by itself and keeps what was said",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "ends"],
      ["report reads", "It * froze."],
      ["undo"],
      ["report reads", "It froze."],
    ],
  },
];
