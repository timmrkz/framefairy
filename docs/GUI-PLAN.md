# GUI plan

The app is built in batches of a couple of minutes each. After a few
batches Claude reports back, and Tim builds and tries the result on his Mac.

Status marks: `[x]` done, `[~]` done in a first version, `[ ]` open.

---

## Decisions

1. **Toolkit.** Go and Wails v3 (pinned to v3.0.0-beta.23) with Svelte 5 and
   TypeScript. One module on Go 1.27 for the engine, the command line, the
   app and the training tool.
2. **Storage.** Plain files, no database. Settings and the episode list live
   in the user's configuration folder. Everything about an episode lives next
   to it in `<episode>.framefairy/`.
3. **Caption store.** `clips.json` holds the captions, on source time. The
   per-clip SRT stays as an export and as the command-line way to correct
   them.
4. **Preview.** One lightweight copy of the episode for playback in the app,
   like optimized media in Resolve. Renders always use the original.
5. **Command line.** Stays, with all its flags, and runs the same engine code
   as the app.
6. **Time window.** The app offers the command line's `--from` and `--to` as
   a window dragged over the episode waveform.
7. **Hardware.** Apple silicon, and no Intel Mac. No language model ships.
   The user picks a model in the cloud, Anthropic's or OpenAI's with their
   own key, or a local model, and the app walks
   them through installing the local one, choosing by available memory.
   Tim's machine is an M2 Max with 32 GB. See [PACKAGING.md](PACKAGING.md).
8. **Licence key.** At the very end.
9. **Training.** The engine records plans and decisions. Everything that
   works with those records is a separate tool, `framefairy-train`. The app only
   shows what serves making shorts: keep and reject, no reason chips, no
   training screens, no dataset export.

---

## Phase 1: engine for the app

| # | Batch | Status |
|---|---|---|
| 1.1 | Structured events from the log, `--events FILE` | `[x]` |
| 1.2 | `Project` with separate steps on top of `Run`, windows cut from the whole transcript | `[x]` |
| 1.3 | Episode status from the files on disk | `[x]` |
| 1.4 | Waveform peaks, silences, words in a window, plan views | `[x]` |
| 1.5 | Thumbnails: any number per clip, picked as marks on the clip timeline, written by the render beside the short as frames of it. Spec in [THUMBNAILS.md](THUMBNAILS.md). In the plan `[x]`, on render `[x]`, on the clip timeline `[x]` | `[x]` |
| 1.5b | The playback copy of the episode, for a file the app cannot play or plays badly | `[ ]` |
| 1.6 | Plan edits: keep or reject `[x]`, trim edges `[x]`, change the cuts inside a clip with snapping `[x]` | `[x]` |
| 1.7 | Caption edits in the plan, with SRT export and import | `[ ]` |
| 1.8 | A clip's captions on request, in the lines and the look the render uses, for the caption preview | `[x]` |
| 1.9 | The model's answer streamed, and each clip taken the moment its last character arrives | `[x]` |
| 1.10 | A clip framed and written to the plan as soon as it is taken, while the model writes the next one, so the first clip shows long before the last | `[x]` |
| 1.11 | Real progress for a search: loading the model, reading the transcript, thinking, writing clip by clip and framing, each measured against how long it took on this machine before, and against a measured stand-in the first time. Shown in the row the next clip will appear in | `[x]` |
| 1.12 | Framing decodes only the parts of a clip that are kept, through the system's own video decoder where there is one, and on every system | `[x]` |
| 1.13 | The local model thinks on a budget, 2,048 tokens by default, so the first clip is written in about a minute rather than five | `[x]` |
| 1.14 | The model loaded while the transcription is still running, so a search that starts finds it in memory `[x]`, and the transcript read into it as it grows `[ ]` | `[~]` |
| 1.15 | The transcription waits while clips are found and carries on after, so the model has the machine to itself | `[x]` |
| 1.16 | A search's row says Waiting for the transcript with its window, then Finding clips, then how many are found, each held long enough to read, with Cancel from the moment the first search is on its way. A fill that can be told from what is left in daylight. Up to four clips framed at once | `[x]` |
| 1.17 | A search starts the moment the window has been heard: the transcription stops exactly at the window's edge, told where by the workspace, and stays there: it runs for searches and for nothing else. New works before the window is transcribed, waits for it the way the first search does, and starts the transcription itself. Cancel stops both, and the pause mark at the transcript's edge is gone. The window is locked from the start of the transcription until the search has run. Paused work keeps its fill and stops moving. The part of the clip timeline not heard yet is grey like a clip card still to come, breathing while the transcription runs and still when it does not | `[x]` |

## Phase 2: app shell

| # | Batch | Status |
|---|---|---|
| 2.1 | App module, the app's window, services, job queue with progress and cancel | `[x]` |
| 2.2 | Layout, navigation, design tokens | `[x]` |
| 2.3 | Settings with the setup check | `[~]` |
| 2.4 | Episode list in the sidebar, add and remove | `[~]` |
| 2.5 | Episode: transcribe, time window, find clips | `[~]` |
| 2.6 | Clips: title, reason, words, keep or reject, preview and render | `[x]` |
| 2.7 | Playback from the episode copy, instead of rendered previews | `[ ]` |
| 2.8 | Activity: progress, cancel, log per job, show in folder | `[~]` |
| 2.9 | First run on Tim's Mac, fixes from his notes | `[~]` |
| 2.10 | Transcription in its own background lane, saved as it goes and resumed after a pause. Finding clips starts as soon as the transcript reaches the end of the chosen window | `[x]` |
| 2.12 | One workspace per episode: player with the crop of the selected clip, clip list beside it, clip marks and playhead on the timeline, play a clip with its cuts. The separate clips page is gone | `[x]` |
| 2.14 | Slim range picker with clip marks, time labels and playhead inside, one search row, no overlap with the player, button-free clip list, Render in the clip panel, keep and reject removed from the app | `[x]` |
| 2.13 | Remove an episode with or without deleting its files, time window locked while clips are found | `[x]` |
| 2.15 | One Play button that plays from the playhead and stops at the end of the clip, space bar for play and pause, the clip's captions drawn in the video preview, the clip list as tall as the video preview, words corrected in the caption box | `[x]` |
| 2.11 | Episode opens with a frame preview and the time window right away, first 30 minutes chosen for long episodes, waveform filling in during transcription | `[x]` |
| 2.18 | Searched parts marked on the range picker, a window drawn only outside them, and a click on a marked part to search it again | `[x]` |
| 2.17 | One box over the workspace for what cannot be taken back: removing an episode, and finding clips again for a window that already has some | `[x]` |
| 2.57 | What a search can ask for, worked out and shown where it is chosen: a window no larger than the model can read in one request, and no more clips than the window can hold at their shortest | `[x]` |
| 2.62 | The settings made presentable: cards in the manner of the Updates page, sharing `.card` with it, **Find clips with** as one list of the Claude API and every local model, grouped by where they run, with the rows that depend on it in the same card, a model not there yet fetched and then used when it is chosen, **Downloaded models** folded, what is in the way said in the row where it is put right, a folder dialog for where shorts go, the Mac's accent colours, the machinery behind **Advanced**, and everything saved as it is changed, with no Save button. Tried first and left: a card of ways and a card of models under it, and a status card at the top | `[x]` |
| 2.63 | More than one company in the cloud: OpenAI beside Anthropic, the model saying which company, each with its own key in the keychain and the environment, OpenAI's answers read as they are written and put into the same shape as Anthropic's, so the retries, the cost, the clips as they arrive and the repair are one path. **Find clips with** lists a model from each company, the key row names the company, and the first run asks for the key of the model chosen. A fake server stands in for both companies in the engine's tests, down to a whole search on a GPT model, the app tests keys per company, and `finding.test.ts` walks every state of the row | `[x]` |
| 2.64 | The API keys kept where only the app may read them: through the Security framework rather than the `security` command, so a key never passes through a command line and the keychain item's access list names Frame Fairy, a key asked for without being read whenever the settings open, and the first keys, which anything could read, moved and removed the first time they are used. Tests never reach the machine's keychain. Every new build asks once for the key until the builds are signed, see 5.5 | `[x]` |
| 2.65 | The settings keep the app where it is while clips cannot be found, a model chosen and not downloaded above all, which Tim was able to walk away from into an app that could not search: a move to another page is refused and the Finding clips card shakes the way the Mac's password field does, with the keyboard on the list. A download on its way holds it too, because the model is no use until it has arrived | `[x]` |
| 2.66 | Updates that do what was picked: a pull request merged or closed downloads nothing, not main by itself, and the page says it is closed and waits for a choice. A pick while a download runs stops that download at once and shows the new channel from the moment of the pick, and nothing the download before says afterwards reaches the page. Found by Tim with #25 and #23 | `[x]` |
| 2.67 | A key the company refuses is found where it was typed: Save key shows it to the company first and says in the key's own row when it is refused. A key refused during a search says so in words and where the key came from, not the company's JSON. The app uses the key saved in its settings before one in the environment. Tim had saved the key's ID, apikey_..., which the check on saving now refuses at once. The app also uses the key saved in its settings before one in the environment, so a key left in a terminal's environment never wins over the one just saved | `[x]` |
| 2.68 | A search in the cloud shows how far it is from its first search: a model the app knows is measured against a first guess until it has been timed, and Anthropic's thinking models send a summary of their thought as it goes, where before the stream was silent until the answer began and the row showed only the beam for minutes. Found by Tim | `[x]` |
| 2.69 | The key row as the consoles show keys: a saved key in short, `sk-ant-api03...MwAA`, read from the keychain item without the key, a narrower field, a refusal in one short line that also covers a key's ID, and a trash can that removes the saved key after asking. A refused key is taken the way the Mac's password fields take a wrong password: the field shakes, keeps what was typed, selected, with the keyboard in it, so the next paste replaces it, and Save waits for something new. Without a key, the row links to the page where keys are made, at platform.claude.com, where Anthropic keeps them now, or platform.openai.com. Asked for by Tim | `[x]` |
| 2.70 | The waveform of the whole episode within seconds of adding it, measured on its own without the speech model, where it came from the transcription and stopped where the first search's window stopped. It fills in from the start as it is measured, an episode added before is measured the first time it is opened and nothing is drawn over it for the part not heard yet. A grey band across the middle for the captions still to come was tried and taken out: Tim saw it as a dark block in the sound, and the range picker already shows what has been heard. Asked for by Tim | `[x]` |
| 2.71 | Faster transcription, measured first: `make speechbench` runs several copies of the speech model side by side, each its own pieces, over every mix of copies and threads that fits the machine `[x]`, measured on Tim's M2 Max, four copies of 2 threads at 83 times real time where one of 8 heard 47, the same words `[x]`, the app hears with as many copies as the machine it runs on has performance cores and memory for, in the order it was said `[x]` | `[x]` |
| 2.72 | A fitted clip takes only an answer about itself: one that carries its slug, or stands in its place under a slug no other clip has. Asked about the mirror story, the model gave the umbrella story back, which was taken for the mirrors by its place, found to repeat the umbrella story, and dropped, leaving the mirrors at 43 s without a word. A clip left out is now named in the log and asked for once more, alone, and the answer is saved as `refit` beside `fit`. The **Clips** setting is now **Target**, with a `title` that says the model gives fewer when fewer moments are strong enough. The info mark of **New clips** says what Target, Shortest and Longest are, and the rule on how long a window may be moved to the range picker's, in plainer words. Asking again for clips missing from a search was turned down: six of eight is a judgement. | `[x]` |
| 2.73 | The waveform is measured where the clip timeline looks first. A playhead put near the end of a four hour episode waited for everything before it. The waveform request says what the clip timeline shows, and the measuring goes there, then on, then from the start, jumping with `-ss` when the view moves to a part not measured yet. The file keeps the parts measured, version 2, so a measuring cut off carries on, and the transcript fills what the measuring has not reached. Starts a fifth of a second early, since the first 40 ms after a seek came out up to 4 dB off. | `[x]` |
| 2.74 | Updates for the way Tim works: the app looks for a newer build the moment it starts rather than five seconds later, a build is named by its short commit rather than the workflow's run number, `0.3.0-pr28.a1b2c3d`, the commit on the Updates page opens on GitHub, and the word Follows in front of the list of channels gives way to an info mark beside it, which says how updates work. A build downloaded once is kept, so going back to a channel does not download it again, and a build the channel list stops naming, a pull request closed or a push replaced, is removed. Found by Tim switching between #29 and #30. The button that restarts into a new build says Relaunch, Chrome's word, and a build that is ready when the app quits goes in on the way out, so the next start is the new build | `[x]` |
| 2.75 | Clip edges land on the frame like the edges of a cut, with shift for whole words, so a clip can be trimmed a frame at a time | `[x]` |
| 2.76 | The caption blocks on the clip timeline follow an edge or a cut while it is dragged, made by the engine from the pieces as they are drawn | `[x]` |
| 2.77 | The playhead goes with a dragged clip edge and the video preview shows the clip as it is dragged. With shift the word the edge snapped to is always lit. The clip edge lies over the caption handles | `[x]` |
| 2.78 | A word is said and captioned while a clip holds any of its sound, one rule for the words a clip keeps, its captions and a drag, so dragging an edge back over a word brings its caption in at the word's last sound. A word a cut parts is captioned once | `[x]` |
| 2.79 | A word the recogniser heard as one keeps its end across a breath inside it: "sweet-grundschulliebe" was cut off before "liebe", which had no caption and was never lit | `[x]` |
| 2.81 | The clip list goes round: shift and the down arrow on the last card is the first, shift and the up arrow on the first is the last. Asked for by Tim | `[x]` |
| 2.80 | Shift stops at the words that light up: the halves of a hyphenated word and a correction that reads as two are two stops, split by the engine from the clip's caption style. A word keeps its own time at the clip's outer edges, so its halves stay where they are said | `[x]` |
| 2.81 | Windows that grow with the episode, as the square root of its length, cut into equal windows of at least 10 minutes: half an hour makes three of 10, four hours eight of 30, so a new episode shows its first clips sooner. The target follows the window, one clip for every twelve clip lengths of a half hour and by the square root for other windows, so six minutes get 3 rather than 1, shown in grey in Target until a number is typed, and cleared to follow again. The local model thinks in proportion to the window. Asked for by Tim, with the twelve from the 6 clips the model gave when asked for 8 | `[x]` |
| 2.82 | Clips made by hand with **I** and **O**, for a moment the model did not pick, rebuilt from #24 on what the app already has. A clip made by hand is heard by the step a search takes, proposed by the playhead and shaped, framed and written by the one plan builder into a clip set that grows and searched nothing. It is a job of its own kind, any number at a time beside a search, placing its crop in a fourth lane. Every job lists the clips it has on the way, and the clip list shows each in its place with the beam until it is written, so a search's clips and a clip made by hand come in the same way. The rule in CLAUDE.md that says a new use goes through the thing there is. Asked for by Tim | `[x]` |
| 2.83 | Open: giving a searched part back frees it to be searched again and takes the search's clips there with it, and leaves the clips made by hand, which searched nothing. Whether that is right, and whether the range picker then says what it frees, is to be looked at again once the clips made by hand are done. Noted by Tim | `[ ]` |
| 2.84 | The transcript heard in parts, where it is needed first: a clip made by hand has the minute around the playhead heard wherever the rest has got to, and a search its window and nothing before it. One transcript file with its parts, each read with a seek that lands on the sample and 3 s either side, and joined with every word once. The range picker shades each part not heard and fills each part heard. Asked for by Tim | `[x]` |
| 2.85 | A target typed is for the window it was typed for: another length, in the workspace or in the first search of a video added, follows its own suggestion. Noted by Tim | `[x]` |
| 2.86 | One word model, see [WORDS.md](WORDS.md): the words said are one list with the corrections in it, a clip keeps no words, captions are made one way and never read back, and every gesture on the clip timeline is worked out by the engine, `ShapeClip` while the hand moves and `Reshape` when it lets go. A clip on its way lays its captions out from its pieces and the words said in them, the same way | `[x]` |
| 2.87 | Episodes in the order they were added, the newest at the foot, or by name, picked from an icon in the head of the sidebar, which stays open while its list is. The library was sorted by path, which lost the order added. Noted by Tim | `[x]` |
| 2.61 | Text and Box in the captions column are switches like Highlight: a short can be rendered without the box, or without captions at all | `[x]` |
| 2.60 | Jobs as one thing each, see [JOBS.md](JOBS.md): the design `[x]`, the path tests `[x]`, the records and `Project.Search` in the engine `[x]`, the app running jobs in three lanes `[x]`, the interface switched over `[x]`, the old parts gone `[x]` | `[~]` |
| 2.59 | A search that stopped before it found anything says so where its clips would have been, with its reason, after a restart too, and points to **New**. Nothing starts by itself | `[x]` |
| 2.58 | Models in the settings: an install carries its fill and Cancel in its own button and says how far, how fast and how long, a model that is there can be removed after asking, and with several installed one is chosen with Use. The install of a language model never showed at all, because its row looked for it under a name the job never had | `[x]` |
| 2.55 | The remove box reads Cancel, Keep, Delete, in plainer words, with what is destructive marked rather than made the answer Enter gives | `[x]` |
| 2.54 | The crosshair goes to the playhead, always, and puts it in the middle. Going back to the clip is what clicking the clip does | `[x]` |
| 2.53 | A ruler is two layers, the line behind and the time in front, so a window over a searched part no longer swallows the minutes it covers, with the shape of it tested rather than only the numbers | `[x]` |
| 2.52 | Nothing a job reports can be a number JSON cannot carry, so a share worked out from a length nobody could measure never silences the app about that job | `[x]` |
| 2.51 | A plan is replaced in one step and under the lock the edits take, so the clip list and the coverage never read one half written, and removing an episode refuses rather than deleting under a job that will not stop | `[x]` |
| 2.50 | The bar is the title bar: macOS is asked how tall it laid it out and where it put its buttons, and is told again whenever it lays it out afresh, so the buttons are centred in the bar by construction rather than by a number | `[x]` |
| 2.49 | An episode opens on the clip it was last worked on, or on the first one, so a workspace with clips in it is never waiting for a click that says nothing | `[x]` |
| 2.48 | The ruler under the window rather than over it, so the edge of the window is whole where a minute falls on it, and the list of faces ending where the numbers end | `[x]` |
| 2.47 | The playhead drawn over the tracks so its head stands above them, and frames that cannot arrive out of order: a file of its own per ask in the engine, a ticket per ask in the interface | `[x]` |
| 2.56 | One way of showing work in hand, everywhere: the beam round the control the work came from, the motes it sheds, the fill for how far it has come, the shimmer over a place waiting to be filled, and the pulse on a dot for work running elsewhere | `[x]` |
| 2.46 | The pane stays about the clips while the transcription carries on behind them, the chosen card kept in view, an editor's playhead in the app's own colour, and progress that fills or travels | `[x]` |
| 2.45 | The clip list as a stack of cards under a veil with the work in the button, an editor's playhead, the arrows walking the clips, and the bar asking macOS where its buttons are | `[x]` |
| 2.44 | Two colours in the settings, the app's own and the word highlight, starting out the same, with the lighter shade and the wash mixed from the app's | `[x]` |
| 2.43 | The layout worked out by the stylesheet from the size of the app, with nothing measured while it is resized, and a waveform drawn one whole pixel at a time so it keeps its weight at every zoom | `[x]` |
| 2.42 | Nothing left over at the foot: the two tracks take the height the picture cannot use, the range picker always half the clip timeline. The clip settings only in the workspace, where they save themselves. `#942192` | `[x]` |
| 2.41 | The transcript edge moves with the work rather than with the saving of it, the window moves off what it just searched, the clip list waits in the shape it will have, and the magnifier is off where there are no words | `[x]` |
| 2.40 | One folder for the training data, named and cleared in the settings, and removing an episode leaves nothing behind. One mark per area, shown on hover. The transcript read again while it grows, so the first search starts | `[x]` |
| 2.39 | The buttons under the clip timeline, a quiet range picker, a window that is an X-ray over searched material, and a picture that always shows the playhead | `[x]` |
| 2.38 | What the app does by itself as plain rules with tests: a new episode transcribes itself, and only an episode nobody has searched finds its first clips by itself | `[x]` |
| 2.37 | An episode whose work folder was deleted by hand starts again by itself, and the remove box has nothing to ask about | `[x]` |
| 2.36 | Room that is there is used: the video preview as big as the height or the width allows, the columns beside it taking the rest, and a sidebar that does not flicker as it opens | `[x]` |
| 2.35 | One place for the captions, kept for every episode: the drag saves the setting, **Height** stands with the caption settings, and the way back with them | `[x]` |
| 2.34 | One gap everywhere in the workspace, and Escape taking the lens away | `[x]` |
| 2.33 | The transcription in the head of the clip list, in the same shape as a search, and places waiting to be filled instead of a pill over the track | `[x]` |
| 2.32 | A bar across the top of the app with the close, minimise and zoom buttons and the name of what is on screen, and info bubbles that are short and stay inside the app | `[x]` |
| 2.31 | A rail whose marks do not move: adding an episode, activity and settings at the foot of the sidebar, in the same place open or closed | `[x]` |
| 2.30 | A window drawn anywhere on the range picker: part of a search can be removed on its own, the trash can rides the window, and looking again at searched material asks first | `[x]` |
| 2.29 | The playback buttons in the row above the clip timeline, nothing under the range picker, a clip list on a surface of its own, a mark instead of **Fit**, and a clip in the list putting the timeline back on it | `[x]` |
| 2.28 | The window on the range picker put right: one box with one border and no bars beside it, and a light passing through it while clips are being found instead of stripes that tiled badly | `[x]` |
| 2.27 | Breaking the app on purpose: every path checked against the library, links not followed out of it, plans that would edit the wrong clip refused, edits of one plan one at a time, and a paused transcription that stays paused | `[x]` |
| 2.26 | A video preview as big as the app allows, the two timelines named, one word for removing, and windows that land on a round step | `[x]` |
| 2.25 | A swipe that stays put, time labels on the clip timeline, the transcript kept in memory, and a removed clip seen going: its row stays in place in red for ten seconds with **Put it back** | `[x]` |
| 2.24 | Two fingers on the clip timeline: a swipe along the episode, a pinch to zoom, and **Fit** to put the view back | `[x]` |
| 2.23 | Removing a search from the range picker, which leaves its part free to be searched again and takes its clips with it | `[x]` |
| 2.22 | The track put right: a click anywhere moves the player, a window never lies across a searched part, and a seek that arrives before the video has read its index still lands | `[x]` |
| 2.21 | The search where the clips appear: its progress and **Cancel** under the head of the clip list, the window boxed on all four sides and striped while it is being searched, and the clips shown as soon as the answer is written | `[x]` |
| 2.20 | The transcription on the range picker instead of a bar of its own, the first search for a new episode starting by itself once the transcript covers the window, the words and the waveform arriving as the transcript grows, and a settings column of one row per setting | `[x]` |
| 2.19 | The clip timeline cleared up: the waveform has the whole track, no words are written over it, and the lens opens and closes on a click of the magnifier, which never moves the playhead | `[x]` |
| 2.16 | A collapsing sidebar that lies over the workspace, the settings it uncovers in a column of their own, **New** above the clip list, a clip removed with the trash can and put back again, help in info bubbles instead of standing text, and the whole workspace fitting the window | `[x]` |

## Phase 3: clip editor

| # | Batch | Status |
|---|---|---|
| 3.1 | Detail timeline under the overview, zoomed to the selected clip, with waveform and words | `[x]` |
| 3.2 | Word track and segment track, cuts shown between pieces | `[x]` |
| 3.3 | Playhead, kept in step with the player, and a timeline that is there even with no clip selected | `[x]` |
| 3.4 | Drag clip edges, on the frame or with alt on words, saved to the plan | `[x]` |
| 3.5 | Cut or restore a pause between two kept words with one click | dropped |
| 3.5b | Lengthen or shorten a pause | dropped |
| 3.8 | Drag the playhead through the timeline, and a click anywhere hands the keyboard over | `[x]` |
| 3.9 | Click a clip edge to put the playhead on it, and loop the clip while it plays | `[x]` |
| 3.6 | Undo and redo for everything done to a clip: trims, cuts, the crop frame, the caption box, words, the caption look, removing a clip and removing a search. One history per episode while the app is open. Jobs, moving around and app settings are not in it | `[x]` |
| 3.7 | Crop frame shown per shot, moved by hand per camera angle, reset to automatic | `[x]` |
| 3.10 | The colour of the caption text, of the box behind it and of the highlight, each with how much of it is seen, beside the face and the size. The caption blocks of the clip timeline wear all three with their opacity, over everything else on the track | `[x]` |
| 3.11 | One frame for what is chosen: the crop, the window and the clip drawn by one rule, with the same line, corners and colour | `[x]` |
| 3.11b | A clip under the hand lit in all three places it is shown: its card, its mark on the range picker, and its mark or frame on the clip timeline | `[x]` |
| 3.12 | Captions stay the size chosen. A word too long for a line gets a caption of its own and is hyphenated with TeX's patterns for the language of the episode, a German compound where its parts join, instead of one long word making every caption of the clip smaller | `[x]` |
| 3.13 | The clip timeline zoomed out: every other clip of the episode drawn as the range picker marks it, a click choosing it, and the caption blocks only once they are wide enough to be read | `[x]` |

## Phase 4: caption editor

| # | Batch | Status |
|---|---|---|
| 4.1 | Captions drawn in the video preview with the render's font, place and highlight `[x]`, compared against a rendered frame `[ ]` | `[~]` |
| 4.2 | Live ASS for the clip being edited | `[ ]` |
| 4.3 | Correct words in the caption box in the video preview `[x]`, a correction that holds several words `[x]`. Split and merge captions dropped: the edges of a caption and the words in it are enough | `[x]` |
| 4.4 | A caption shown earlier or later, and for longer or shorter, where the timing of the words is a little off from what is heard | `[x]` |
| 4.4b | Drag the caption box up and down on a grid, per clip, with one click back and one click to undo that | `[x]` |
| 4.5 | The caption face and size next to the clip `[x]`, the highlight colour in settings `[x]`, the rest | `[~]` |
| 4.5b | The word highlight turned on or off per clip set, with its bounce, in the short, the video preview and the clip timeline. The name of the row is its switch, struck through when off | `[x]` |
| 4.6 | The caption fonts built into the binary, so nothing has to be installed | `[x]` |
| 4.7 | Lines broken by measured width, and the size brought down when a word will not fit | `[x]` |

## Phase 5: packaging

macOS first, because it is the machine that can be tested. The reasoning
behind all of it is in [PACKAGING.md](PACKAGING.md).

| # | Batch | Status |
|---|---|---|
| 5.0 | The speech library carried in the bundle: the runpath off the build machine's module cache and onto the app. Done on Linux first, and the macOS half was doing nothing at all until the bundle made it visible | `[x]` |
| 5.1 | Licence memo: which ffmpeg, and what H.264 means for a paid app | `[x]` |
| 5.2 | `Frame Fairy.app` built by `make app` and started by `make run`, so what Tim runs every day is what a customer runs: the same `Info.plist`, the same privacy prompts, the same tools inside the same folder. Wails' own Taskfile layout is not adopted, see [PACKAGING.md](PACKAGING.md#how-the-building-works). The icon is one PNG in `build/`, made into an `.icns` by the build | `[~]` |
| 5.3 | An LGPL ffmpeg built without libx264 `[x]` for macOS, encoding through the system `[x]`, found next to the app before the search path `[x]`. Linux still has no encoder in an LGPL build, so VA-API or openh264 goes in before Linux ships | `[~]` |
| 5.3b | The llama-server we ship, built from llama.cpp, so choosing a local model is not an instruction to go and install something | `[x]` |
| 5.3c | The tools we ship built by a workflow started by hand, kept as an archive with a manifest read off the binaries, their source published beside them, and a checksum anybody can hold a copy against | `[x]` |
| 5.4 | The first run: the speech model fetched by the app itself `[x]`, the one question asked once and remembered `[x]`, an Anthropic API key stored in the keychain `[x]`, four language models from three houses, each pinned to a checksum read off the real file, judged against what the machine can hold, worked out from how each model attends rather than from its file, with the one it should have marked `[x]`, the same lists in the settings `[x]`, a download that stopped carried on rather than started again `[x]`, and `llama-server` found beside the app before the search path, with the local way not called ready without one `[x]`. | `[x]` |
| 5.4b | The licence of every piece of other people's work the programs are made of, generated from what they are built from and checked by a test and by the interface build `[x]`, shown under **Help → Acknowledgements**, where apps keep them `[x]`, beside ffmpeg and llama-server `[x]`. The speech library in the Go module carries espeak-ng, which is GPL 3, so what ships is sherpa-onnx's own release without speech synthesis, pinned for Apple silicon and Linux `[x]`, Windows still to pin `[ ]` | `[~]` |
| 5.5 | macOS signing and notarisation, every bundled binary included. It also ends two questions macOS asks after every new build: Documents again, and whether the app may use the API key it keeps in the keychain, because each unsigned build is a new app to both | `[ ]` |
| 5.6 | Windows and Linux builds in CI | `[ ]` |
| 5.7 | Whether customer builds record training data, and a setting for it | `[ ]` |
| 5.8 | Licence key check and storage | `[ ]` |
| 5.9 | How the app updates itself. Researched in [UPDATES.md](UPDATES.md): what an update replaces and what it never touches, the ways it is done, how a licence decides who gets which, what the person sees, and the release workflow. Decided: a licence gets every update for ever, Wails' updater with a channel list of our own, and the code public with the builds in this repository's releases `[x]`. Updates for pull requests, for Tim to use every day: the channel list and its checks, the signing tool, the workflow that builds main and every pull request, Updates as the last row of the sidebar, with the build running and a dot when a newer one is ready, and a page of its own, one card with the build, the channel it follows, where things stand, and Check or Update, `make install` and `make update-key` `[x]`. Tried on Tim's Mac: #18 downloaded, swapped and restarted `[x]`. macOS asks for Documents again after every update until the builds are signed with Developer ID `[ ]`. Installing when the app quits, the way Chrome does, built `[x]` and tried on the Mac `[ ]`. Customers: the stable channel, signing, the release key `[ ]` | `[~]` |
| 5.9b | A build for every commit pushed to a pull request, also while it is in conflict with main: built on the push, once per commit, and the Updates page says when a newer commit is still being built, looking every twenty seconds until it lands | `[x]` |
| 5.10 | A skill for shipping, once a build has actually been through signing and notarisation. Not before: a skill written from reasoning rather than from a round of it would teach the guesses | `[ ]` |

---

## Robustness track: the glue never breaks

The engine, the app's Go side and the interface hand work to each other
all the time: a transcription saves while a search reads, a job reports
while the window listens, a model is held by one job and let go by
another, a plan is written by a search while a person edits it. Each of
these handoffs is an assumption about order and timing. The rule for
this track: **a slower or poorer moment is fine, a frozen, broken or
stuck app is not.** Whatever goes wrong ends one piece of work with its
reason and leaves the app able to go on.

Every batch finds a handoff, writes the test that makes its failure
happen, under `go test -race` and from several goroutines at once, and
then makes the code survive it.

| # | Batch | Status |
|---|---|---|
| R.1 | An audit of every handoff between engine, app and interface, ranked by what it can break, in [ROBUSTNESS.md](ROBUSTNESS.md) | `[x]` |
| R.2 | The job queue under everything the app can do to it at once: add, find, list, cancel, remove an episode, quit, while jobs run, report and panic. Removing an episode closes it to new work, a panic around a job ends that job and not the lane, a job's news can arrive in any order and the newest wins, quitting stops every job `[x]` | `[~]` |
| R.3 | Transcripts: a save that is cut short, read while it is written, a pause that lands mid-save, carrying on from a file that is damaged | `[x]` |
| R.4 | The model host: loads that fail, hang or are stopped, holders that let go twice or never, the app quitting while a model loads. One model in memory at a time, quitting stops the jobs and the model even while it loads, a server left by an app that crashed is stopped at the next start, and Cmd+Q on a Mac, where app.Run never returns, stops the model from a shutdown hook. llama-server is killed half a second after it is asked to go, so Cmd+Q and removing an episode no longer freeze the app, and Cmd+Q answers at once, asks first while work runs and stops everything away from the main thread | `[x]` |
| R.5 | Plans written by a search while they are edited, undone and removed. A clip that lands during an edit is not the edit's `[x]` | `[~]` |
| R.6 | The interface: answers that arrive late, out of order or for an episode no longer shown, events that stop, promises that never settle. Job news in any order, clip lists, the episode's state and what was searched keep the newest answer `[x]`. The frame read from the file while the video preview catches up is the frame the video shows there, not the nearest second | `[~]` |
| R.7 | What the person sees when something fails: a reason in words, and a way on | `[ ]` |
| R.8 | A way for a customer to tell us what went wrong, with what we need to fix it. Today there is none: the app keeps no log of its own, and what a search sent and got back sits in the episode's `logs` folder, where nobody would look. Proposed: a log of the app's own in `~/Library/Logs/Frame Fairy/`, where Mac apps keep theirs and the Console app shows them, and **Help → Report a Problem…**, which writes a report of the build, the macOS version, the settings without any key, the checks and the recent log, and opens an email with it ready to attach. Waits for the address reports go to. Found when a key was refused and there was nothing to send | `[ ]` |

---

## Clip choice track: better stories, better cuts, less waiting

How the model is asked, and what the engine does with its answer. Every
experiment runs from the command line with `--compare`, see
[CLI.md](CLI.md#trying-other-ways-of-asking), and is kept or taken out by
what Tim reads and what the report counts.

| # | Batch | Status |
|---|---|---|
| C.1 | Recipes, `--compare` with a report of what each cost, found and got wrong, the same seed on every side | `[x]` |
| C.2 | The engine around the answer: numbers kept apart from words, every edge on a whole sentence, one clip per moment, clips far off the length asked for again | `[x]` |
| C.3 | A brief for any video, what must never be cut, the task said again after the transcript. The app's default since prompt version 3 | `[x]` |
| C.4 | A yardstick: about 40 stories whose first and last sentence and what to leave out Tim has marked, scored by the program over three seeds | `[ ]` |
| C.5 | Decided with the yardstick, not by one run: 1024 tokens of thinking, the shorter `stories2` transcript, worked examples from Tim's cuts, a close second look at each clip's edges | `[ ]` |

Tried and taken out: asking the model to review its own edges, telling
the voices apart, cutting an overlong clip automatically. The reasons are
in [ENGINE.md](ENGINE.md#recipes).

## Training track, separate from the app

Follows `framefairy-training-plan.md`, with one change: nothing of it appears in
the app.

| # | Batch | Status |
|---|---|---|
| T.1 | Prompt version, `plans.jsonl` for every new answer, `plan_id` in the plan | `[x]` |
| T.2 | `decisions.jsonl`, line mapping from segments, keep and reject record themselves | `[x]` |
| T.3 | `framefairy-train mark` with reasons, published and unpublished | `[x]` |
| T.4 | `framefairy-train export` with SFT, DPO, episode split and manifest | `[x]` |
| T.5 | Trims record `edited`, renders record `rendered`, every decision carries its changes against the proposal | `[x]` |
| T.5b | No double counting: same answer keeps its record, export merges by prompt and by proposed clip, episodes known by content | `[x]` |
| T.5d | `viewed` records, and clips played but not rendered as weak negatives in the export | `[x]` |
| T.5c | Answer format v2 in which the model can cut a pause between two lines, so pause edits become targets. Spec in `docs/TRAINING.md` | `[x]` |
| T.6 | `framefairy-train import` for already published clips | `[ ]` |
| T.7 | `framefairy-train eval` with hit rate on held-out episodes | `[ ]` |
| T.8 | `--llm-lora` passed to llama-server and recorded as the adapter | `[ ]` |
| T.9 | A separate training app, only if the command line turns out to be too little | `[ ]` |
