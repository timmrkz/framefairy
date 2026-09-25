# CLAUDE.md

Guidance for Claude when working on this repository. Read it fully before the
first change of a session. The user is Tim.

## The project

`framefairy` turns a long podcast episode into finished vertical shorts for
YouTube Shorts and Instagram Reels. It started for Tim's own podcast,
*My First Memory*, with episodes in German and English, and is meant to
become a paid desktop app, unlocked with a licence key.

What the product has to be:

- **A black box first.** The episode video is the only input. No captions
  file, no timecodes. Transcription, word timing, clip choice, framing and
  captions happen by themselves.
- **Local.** Transcription always runs on the user's machine. Choosing clips
  is the one thing they pick between: an Anthropic API key, which works on
  any machine and costs per episode, or a local model, which is free per run
  and needs the machine for it. No language model ships, so the app walks
  them through installing the local one. See
  [docs/PACKAGING.md](docs/PACKAGING.md).
- **Faithful.** Audio and picture stay as close to the original as possible.
  Only cutting and burned-in captions, no volume or colour changes.
- **Crisp clips.** A good short cuts fluff and dead air inside a moment
  instead of copying a linear stretch.
- **Light manual control.** The app shows the clips before rendering and lets
  the user trim edges, correct words, place the crop and the captions, and
  change the cuts inside a clip, like Resolve's subtitle tools, and nothing
  heavier. The engine still proposes every cut, and what it proposes is
  right often enough that most clips are never touched. But a proposal is
  not a verdict: a cut can be moved, put back or made by hand, because the
  one thing the engine cannot hear is what the episode is about.
- **For many people.** Native on macOS, Windows and Linux, not tuned to one
  Mac. macOS ships first, because it is the machine that can be tested, and
  on Apple silicon only: an Intel Mac would cost every binary twice and is
  the wrong machine for a product that runs a language model from memory
  through the media engine. A local model is chosen by what the machine's
  memory can hold.
- **Improving over time.** Recorded decisions train our own local selection
  model. See [docs/TRAINING.md](docs/TRAINING.md).

Defaults that were chosen on purpose: 12 clip candidates per search, 20 to 30
seconds per clip, a bouncing word highlight in purple `#942192`,
Gemma 4 26B A4B as the local model, Parakeet TDT 0.6B v3 for speech.

## Where things are

Start with [README.md](README.md). In short:

| Part | Code | Docs |
| --- | --- | --- |
| engine | `engine/`, `asr/` | [docs/ENGINE.md](docs/ENGINE.md) |
| command line `framefairy` | `cmd/framefairy/` | [docs/CLI.md](docs/CLI.md) |
| desktop app `framefairy-app` | `cmd/framefairy-app/` (Go), `frontend/` (Svelte) | [docs/APP.md](docs/APP.md) |
| training tool `framefairy-train` | `cmd/framefairy-train/`, `train/` | [docs/TRAINING.md](docs/TRAINING.md) |
| build | `Makefile`, `scripts/` | [docs/BUILD.md](docs/BUILD.md) |
| shipping | not yet | [docs/PACKAGING.md](docs/PACKAGING.md) |
| plan and status | | [docs/GUI-PLAN.md](docs/GUI-PLAN.md) |

## How Tim works

- He uses Claude through claude.ai, in chat and in cloud sessions at
  claude.ai/code. He does not run Claude Code on his Mac. Changes reach him as
  pull requests on GitHub. After a merge he runs `git pull && make run`.
  Never hand him zip files or ask him to copy files around.
- **Every piece of work becomes a pull request, always.** A branch he has to
  find himself is work he cannot see. Work with no pull request is work that
  has not been handed over.
- **The pull request comes first, not last.** Push the first commit and open
  the pull request straight away, without being asked and without asking,
  and then keep pushing to it. The first commit does not have to be worth
  looking at and does not have to work. It exists so the pull request
  exists, because that is where Tim follows the work as it happens. Waiting
  until there is something good to show means he watches nothing for an hour
  and then gets everything at once. A pull request opened late is the same
  mistake as no pull request at all.
- His machine is an M2 Max with 32 GB of memory, on the latest macOS, with
  Go 1.27, Homebrew and the models in `~/.framefairy/models`. ffmpeg and
  llama-server are built by `make` and live in `bin/`, which is where the
  programs look first.
- He tests the app himself. The cloud VM cannot show the window, so after each
  change say exactly what to look at and what should happen.
- He answers in English or German. Reply in the language of his message.
- Measurements are metric.

## How to work with him

- **Think first.** Research and check before answering. Read the code and
  docs you are about to change. Verify facts about tools and versions instead
  of assuming them.
- **Small batches.** Split larger work into batches of a few minutes each.
  Finish, test and report after each one, with a pull request per batch or a
  commit per batch on one branch. Never disappear into a long stretch of work.
- **Just do small things.** Small, low-risk UI changes need no discussion of
  internal choices. Discuss constraints only for security, lasting effects or
  side effects he would not expect.
- **Ask at most one question at a time**, and only when the answer changes
  what gets built.
- **Report plainly.** What changed, what to test and how, what is still open.
  Name bugs you found on the way, including your own.
- **Watch a pull request, quietly.** Subscribe to its events and act on them:
  a failing CI run is still your work, so is a review comment. But never set
  up a recurring check, never poll, and never write a message that says
  nothing happened. Tim comes back when he is ready.
- **When main moves, merge it in.** A comment from the Main moved workflow
  on your pull request means main has landed something. Merge main into
  the branch, resolve what conflicts, run the checks the change needs and
  push, without being asked. See [docs/WORKFLOW.md](docs/WORKFLOW.md).

## Writing rules

These apply to everything written for Tim: interface text, docs, commit
messages, pull request text, code comments and chat replies.

- **Never use semicolons.** Use commas and full stops. Semicolons in code
  syntax are fine, in prose and comments they are not.
- Plain, direct language. Short sentences.
- **One name per thing.** The area the episode plays in is the **video
  preview**, never the picture or the player. The slim strip under it, the
  whole episode at a glance, is the **range picker**, and the part of the
  episode chosen on it is the **window**, never a stretch. In the
  interface and in the code comments, where the app's own window is meant,
  it is the app. The waveform below the workspace, the episode up close,
  is the **clip timeline**. The key is
  the **space bar**. Taking something away is **remove**, everywhere, in
  every button and every message. Whatever a thing is called in the
  interface, it is called that in the docs and in the code comments too.
- Docs live in `docs/`. Only `README.md` and this file sit at the top.
- When behaviour changes, update the matching doc in the same change, and the
  status in `docs/GUI-PLAN.md`.

## Interface rules

- **The interface is the mechanism, not the machinery.** The app exists so
  a person can turn an episode into shorts they are happy with. Every
  control has to earn its place by serving that, in the simplest form that
  does. What the engine needs in order to work is not a feature: a number it
  computes, a state it keeps, a step it takes, none of that belongs on
  screen just because it exists. And whatever a person can do with one
  click has to be undoable as easily. A control that fails either test does
  not go in, and comes out again if it is already in.
- **Help waits until it is asked for.** No line of instructions sits on
  screen for good. What a control is for goes in its `title`, and anything
  longer goes behind a small info mark that opens a bubble on hover or on a
  click. Text that is always there is text nobody reads, and it moves the
  controls around as it comes and goes.
- **A row never changes width as you use it.** A label that toggles, like
  Play and Pause, gets room for the longer word, so nothing wraps and
  nothing jumps.
- **Controls in one row have the same size.** No one-off sizes for a single
  button. All controls use `--control-h` from `frontend/src/app.css`.
- **What cannot be taken back asks first**, in the one box over the
  workspace, `Confirm.svelte`, and nothing else uses a box. It opens on the
  safe answer and its buttons take the arrow keys and Tab, because macOS only
  tabs between buttons when full keyboard access is on. The buttons read
  left to right from the safest to the one that cannot be taken back, and
  the destructive one is never highlighted and never where the keyboard
  starts: it wears the colour of a warning instead, so it is read before it
  is clicked. Everything else saves at once and is undone with a click.
- **A click shows at once.** Never let a control sit as if nothing happened
  while a job starts or stops. The control says what it is doing and takes no
  second click, and it keeps room for the longer wording so the row does not
  move.
- **What is taken away is seen going.** A row that is removed keeps its
  place for a moment, says what became of it and offers itself back, and
  only then does the list close over it. Nothing vanishes under the
  pointer. Something undoable never asks first, it shows what it did.
- **What the app can do by itself, it does.** Adding a video is enough: the
  transcription starts, and the first search follows as soon as the
  transcript covers the window. A state every episode passes
  through, like having no transcript yet, is never reported as a failure.
- **One space between two things.** Every gap in the workspace is `--gap`
  from `frontend/src/app.css`: between the columns, between the video
  preview and the range picker under it, between one row and the next. The
  space between the window and what is in it is `--edge`. Any other number
  needs a reason that can be said out loud, and inside a control is the
  usual one. A measurement that JavaScript has to agree with, like the
  height the video preview may have, uses the same number and says where it
  comes from.
- **Room that is there is used.** A bigger window makes the interface
  bigger, never emptier. No strip of nothing appears anywhere, so before
  anything is laid out it is decided what may grow and what may not, by
  what the thing is: a picture grows until it runs out of height or width,
  whichever comes first, and keeps its shape. A list, a text and a
  waveform grow with the room. A name with a small field beside it, a
  button, an icon and a rail do not, they stay their size and let the
  space go to whatever may have it. What may grow shares what the rest
  leaves, and the sum is always the whole window.
- **The layout is the stylesheet's work, not JavaScript's.** The browser
  lays the window out in the same pass as the resize. JavaScript that
  measures a size, works another size out from it and writes that back is
  a round trip that lands a frame or more late, and two parts that measure
  each other never settle in one pass at all, which is what a dragged
  window edge looks like when it feels laggy. So every size is one
  expression in the stylesheet, from the window itself, `100dvh` and
  `100dvw`, and the tokens in `app.css`. What the stylesheet cannot know,
  like the shape of the episode, is one custom property set where it
  changes, and it must be something that does not change because the
  window changed. Nothing in the resize path measures anything. A
  measurement is only ever read, never turned back into a size: a canvas
  being told how many pixels it has is fine, because nothing is laid out
  from the answer.
- **Everything lands on a whole pixel.** A row is a whole number of pixels
  high, an element is as tall as itself and not as tall as the line of
  text it stands in, and a height worked out from the window is rounded.
  Something sitting between two pixels is painted in one place normally
  and another the moment opacity, a transform or a filter puts it on a
  surface of its own, because that surface starts on a whole pixel, so it
  steps as it fades in. The same on a canvas: a mark half a pixel wide is
  drawn at half the colour, so a drawing works in device pixels and rounds.
- **What is happening is shown while it happens.** A control that is being
  dragged, a number it stands for, a picture it moves: they all follow the
  hand as it moves, not when it lets go. Anything that shows the same
  thing in another place is kept up to date on the way, and what is saved
  is saved at the end. A number that only catches up when you let go is a
  number that was wrong for as long as you were looking at it.
- **One mark explains one thing, where that thing is.** An area that needs
  explaining carries its own info mark in its top right corner, and the
  mark only appears while the pointer is on that area. No mark explains
  two areas, and nothing explains itself in a bubble that belongs to
  something else.
- **Work in hand looks the same wherever it is.** There is one way of
  showing that something is running and one way of showing how far it has
  come, and every part of the app uses them: the beam round the control the
  work was started from with the motes it sheds, the fill for how far, the
  shimmer over a place waiting to be filled, and the pulse on a dot for work
  running somewhere else. They are in `Busy.svelte` and `app.css`. A new
  kind of loading is not a new animation, it is one of these five in a new
  place. Work that is paused keeps its fill and stops moving, it does not
  disappear. **An animation used in two places is one component, used, never
  copied**: a copy made to look alike drifts the day either is changed,
  and then the two do not look the same. A change to how work looks is
  made once and is true everywhere.
- **One frame for what is chosen.** The crop in the video preview, the
  window on the range picker and the clip on the clip timeline are one
  thing in three places, drawn by `.frame` in `app.css` with the same
  line, corners and colour, and lit the same way under the hand.
- **One list to pick from.** Every list a person picks from in the app is
  `Pick.svelte`, and there is no `<select>` anywhere. A `<select>` is drawn
  by the system: on macOS the webview hands the whole list to AppKit, which
  paints it white with a blue row in the middle of a dark workspace, and no
  stylesheet can reach it. `Pick.svelte` is the app's own list, built on
  bits-ui, which brings the keyboard, the roles, the focus and the floating
  placement and brings no look at all. Nothing about how it looks is decided
  anywhere but in that file and the tokens in `app.css`. A list hangs on
  the edge of the trigger its column reads from and grows away from it, so
  no name is ever cut short in the list and the edge stays one line down
  the column.
- **Consistency over novelty.** A visual treatment used in one place must be
  used for every equivalent element, or not at all. Reuse existing patterns,
  for example the `--ink-3` background for a selected row, before inventing
  new ones. Two things of the same kind are the same size, in the same
  colour, in the same place: the head of a group of settings and the head
  of the clip list are both the head of an area, so they match, and a
  difference between them would read as a hierarchy that is not there.
- Dark workspace, neutral greys, one accent, the system font, tabular
  figures for times. Colours and sizes come from the tokens in `app.css`.
  The accent is `--accent`, `#942192` until it is changed in the settings,
  and the lighter shade and the wash are mixed from it, never set by hand.
  The colour burned into a short is a different setting from the colour the
  app wears, and neither follows the other.
- The episode workspace is the centre of the app, in three columns: the
  settings for finding clips and for the captions on the left, the video
  preview with the crop frame and the range picker in the middle, the clip
  list on the right with **New** above it. Under all three, the clip panel
  with the zoomed timeline. The words of a clip are written out in one
  place, the caption box over the picture, which is also where they are
  corrected. The whole of it fits the window without scrolling.
- The sidebar is a rail until the pointer reaches it. Open, it lies over the
  settings column rather than pushing the workspace about, so the video
  preview never changes size. It stays open when it is pinned.
- **The product only serves making shorts.** No training screens, no reasons
  to pick, no dataset export in the app. Render is the action that matters.
  Removing a clip is a product action, not a verdict to record by hand: the
  engine reads what it means by itself.
- Everything saves at once. No save buttons in the workspace.

## Engineering rules

- **Go, latest version.** One module, `go 1.27` in `go.mod`, no per-part
  version exceptions. Upgrade with Go releases.
- **One command, and the programs still install nothing.** `make` is the
  whole of it: it installs the tools this machine is missing, builds the
  two programs we ship beside the app the first time, ffmpeg and
  llama-server, and builds the programs. Nobody should
  have to remember a second command to get from a fresh machine to a
  running app. A build runner never installs: `CI` in the environment
  turns that off, so what CI builds is what its own workflow asked for.
  The programs themselves never run a package manager. The models are the
  app's to fetch, on its first run, because that is what a customer does,
  and `make models` is left for the command line, which has no window to
  ask in.
- **Build with make.** `make` must finish without any warning on macOS, see
  [docs/BUILD.md](docs/BUILD.md). CI fails on any warning in the macOS build.
  It must also stay quick when there is nothing to do: everything `make`
  decides before it builds is a `command -v` or a file test, never a
  package manager asked what it has.
- **One engine, two front ends.** The app drives the engine through
  `engine.Project`, which calls the same `Run` as the command line. Never
  duplicate engine logic in the app. Keep every command-line flag working.
- **Plain files, no database.** Settings and the episode list in the user's
  config folder, everything about an episode in `<episode>.framefairy/`.
- **Plan edits** go through `editPlan` in `engine/edit.go`, which keeps key
  order and unknown fields, writes atomically and validates with `LoadClips`.
  An edit to a clip removes its caption file, so the next render rebuilds it.
- **Untrusted input.** Model answers, plan files and caption text are
  untrusted. Keep the checks in `ReadPlan`, `LoadClips` and `SafeChild`.
- **Every path is checked against the library.** The app reads, writes,
  renders and serves only files that belong to an episode in its list, and
  it judges a path by where it really leads, not by what it is called, so a
  link inside a work folder is no way out of it. A call that names anything
  else is refused, and one that would have been a job comes back as a job
  that failed with the reason.
- **A job that goes wrong fails itself, not the app.** Work in the queue
  runs behind a recover: a panic ends that job with its reason, and the
  window, the other lane and whatever is being transcribed carry on.
- **What watches running work does not watch the job.** The Go side sends
  a job event about once a second while anything runs, and every event
  replaces the job object in the store. An effect that mentions a job is
  torn down and set up again that often, so a timer inside one never
  fires: that is how the app stopped reading the transcript as it grew,
  and with it stopped finding the first clips. Anything that has to keep
  its own time is started once, in `onMount`, and asks when it fires
  whether the work is still running.
- **What runs on its own goroutine is tested from several at once.** Every
  Go test runs under the race detector, `go test -race`, and anything
  asynchronous, the job queue above all, has a test that does everything
  the window can do to it at the same time: add, list, find, clear, cancel,
  while jobs run and report. A test that makes one call at a time proves
  nothing about a queue. A race only shows on someone else's machine, and
  by then it is their bug.
- **Tests** need no model and no network. Use the fake recogniser and the
  fake llama-server in `engine/project_test.go`. Tests that render skip
  without ffmpeg. Run `make test` before every push **that touches Go**. A
  change only to `frontend/` or `docs/` runs `make interface`, which is that
  type check and the interface's own tests, and builds the preview, and
  nothing else: the Go tests fuzz for ten thousand executions a target and
  take minutes, and no line of CSS can move them. CI runs everything
  anyway, on both systems.
- **Fuzz targets** cover what reads a model answer, a plan file or a caption
  file. `make test` fuzzes every one of them for `FUZZTIME` executions, 10000
  by default and the same in CI, as many targets at a time as the machine has
  cores, see [docs/BUILD.md](docs/BUILD.md).
- `gofmt`, and `go vet` must pass. Log and error helpers take constant format
  strings, pass text as `"%s", text`.
- The interface is built by make into `cmd/framefairy-app/dist/app/`, which is
  not in the repository. Only `cmd/framefairy-app/dist/.keep` is, so Go can embed
  the folder and `go vet ./...` works before the first build. A program built
  without the interface shows a page that says to run make. Never commit
  build output, and that includes `frontend/preview/dist/`.
- Pin Wails exactly (currently v3.0.0-beta.23) and keep
  `@wailsio/runtime` in `frontend/package.json` on the same version.

## Training data rules

Full specification in [docs/TRAINING.md](docs/TRAINING.md). The essentials:

- The engine records every new model answer and every action on a clip in
  `<episode>.framefairy/training/`. It stays on the machine.
- **Never count a signal twice.** The same answer to the same prompt keeps
  its record. The export merges by prompt and by proposed clip and uses only
  the latest state. Episodes are known by content, not by file name.
- **Signals, strongest first:** published, rendered (a full hit when
  unchanged), edited or kept, played but passed over, rejected. Trimmed
  edges become correction pairs.
- The answer format is versioned by `PromptVersion`. Raise it whenever the
  prompt or the line rules change, and keep each version a superset of the
  one before.
- Everything that works with the records lives in `framefairy-train`, never in
  the app.

## Cloud sessions

Sessions at claude.ai/code run on Ubuntu 24.04 on x86_64, with the setup
script from `scripts/cloud-setup.sh` pasted into the environment. It
provides Go 1.27, ffmpeg, and the GTK and WebKit packages the app needs to
compile. There are no models in the cloud, which the tests do not need.

- If `go version` does not show 1.27 or `make check` reports missing build
  tools, run `bash scripts/cloud-setup.sh` and read `/tmp/framefairy-setup-*.log`.
- Run `make test` and `make` before pushing anything that touches Go. Both
  must pass. A change only to `frontend/` or `docs/` does not run them, see
  the tests rule above: waiting minutes on a fuzz run that no line of CSS
  can move is Tim waiting.
- The app cannot be started there, so there is no way to look at the
  window. Interface work goes through the skill in
  `.claude/skills/interface/`, which has the preview harness in
  `frontend/preview/`, what to measure and how to prove a fix instead of
  claiming one. Read it before changing anything in `frontend/`.
- Work on a branch, open a pull request, and let CI run. CI builds and tests
  on Linux and on macOS, where the build has to be clean of warnings, and
  fuzzes on both. It is six jobs at once rather than one after another, so
  the answer comes back in the time the slowest takes, and on a pull request
  each one asks `scripts/ci-needs.sh` whether the change gives it anything
  to do. A push to main narrows nothing. `make test` runs the lot locally,
  and `make unit`, `make fuzz` and `make interface` are the three parts of
  it, see [docs/BUILD.md](docs/BUILD.md).

## Open work

The batch plan with statuses is in [docs/GUI-PLAN.md](docs/GUI-PLAN.md).
Next up, roughly in this order:

- Tim's feedback from testing the current workspace
- splitting and merging captions
- the playback copy of the episode and clip thumbnails
- `framefairy-train import` and `eval`, and loading a trained adapter
- packaging, macOS first: the speech library carried in the bundle, an LGPL
  ffmpeg encoding through the system, signing and notarisation, the choice
  between an API key and a local model. The reasoning is in
  [docs/PACKAGING.md](docs/PACKAGING.md)
- the licence key, last of all
