# The app keeps what was made of a video

A spec, written before any code. The pull requests that build it name
this record.

## In 30 words

The app keeps a notebook for each video in its own folder, named by a
fingerprint of the video's content. Removing, renaming, moving or
deleting the video never loses it. Adding the same file again opens it.

## The problem

The user, on 24 November 2026: "when I remove it and re-add it again
later, the information about the clips that I have already generated
and that I have already rendered are gone ... I then have to go through
the entire video again ... the episodes that were already rendered are
more important to me than the ones that were generated randomly." Asked
how this could hold "even if all of the content and the folders and the
videos themselves are deleted", they chose the design where the app
keeps the clips itself.

What happens today, read on main at ba5b224 and in the logs of the
user's own reports:

- Everything about a video lives in a folder beside it,
  `Folge 12.framefairy/` beside `Folge 12.mp4`. `engine.WorkDir` works
  the folder out from the video's file name. 45 places in the code call
  it, and 23 more call `LogsDir`, which is built on it.
- **Remove** opens a box with three buttons: Cancel, Keep and a red
  Remove. The red Remove deletes the whole folder, `Machine.DeleteWork`
  in `engine/frames.go`: the transcript, every clip set, every word that
  was fixed, and the renders.
- In the user's logs from 7 to 21 November there are 14 removals. Every
  one deleted the folder, `work_deleted: true`. Keep was never chosen.
  Someone who wants to remove a video presses the button that says
  Remove.
- **Save clips to** says Next to each video. The app then renders into
  the `out` folder inside that same folder, `shortsFolder` in
  `engine/run.go`. The user's setting is that one. So the red Remove
  deleted their rendered shorts too, not only the record of them.
- A video renamed or moved in Finder is looked for under a new folder
  name, finds none, and starts from nothing. Its old folder stays behind
  where nobody looks.
- The training records already know a video by its content,
  `EpisodeKey` in `engine/training.go`: the file's size and its first
  and last megabyte, hashed. They are for training and the app never
  reads them, see CLAUDE.md.

What a person made of a video, and what the app can make again, sit in
one folder today:

| Made by a person, or costly to make again | Can be made again in seconds |
| --- | --- |
| the transcript, `logs/words.json` and `words.frames` (minutes to hours of hearing) | what ffprobe said, `probe.json` |
| every clip set, `logs/clips*.json`, with each clip's edges, cuts, word fixes, removed words, crop and where its short went | the loudness of the clip timeline, `levels.json` and `levels.frames` |
| the model's answers, `logs/reply-*.json` (paid for with a cloud key, minutes with a local model) | the caption files, `captions/` |
| the clip and window chosen, `chosen.json`, and the label, `label.json` | the preview renders, `preview/` |
| the jobs that stopped and can Continue, `jobs/` | the prompts and the model's own log in `logs/` |

## What a person sees once it is built

1. **Remove a video.** It leaves the list at once and its row offers
   itself back for a moment, the way every removed row does. No box
   asks, because nothing is lost. Add it again, tomorrow or in a month,
   and the transcript, the clips and the rendered marks are all there.
2. **Rename or move the video in Finder,** then add it from its new
   place. Everything is there.
3. **Delete the video, and the shorts,** and download the same file
   again later. Add it, and everything is there, the rendered clips
   marked as rendered.
4. **A video exported again** is a different file, so it starts from
   nothing. A later step can offer the clips of a removed video with the
   same name and length, see What is left for later.
5. **Free the space.** The settings list the removed videos the app
   still remembers, with what each takes on disk, and **Forget**. Forget
   cannot be taken back, so it asks first, in `Confirm.svelte`. It is the
   one place the app throws away what a person made.
6. **Shorts** go into a folder of their own that a person can see. The
   app never deletes a short.

## Five simulations

### 1. Where state lives: the whole work folder moves into the app's folder

The principle: what belongs to a video is kept where the app keeps its
own things, not beside a file it does not own. Every work folder moves
as it is to `~/Library/Application Support/FrameFairy/videos/<key>/`, and
`<key>` is the fingerprint, sixteen letters and digits like `3f9a1c7e04b2d8a1`.

Layers: the app's library over `WorkDir(key)` over the files. What
moves: the 45 callers of `WorkDir(source)` and the 23 of `LogsDir`
take the key instead of the video's path. The key has to be known
before any path, so it is worked out when a video is added and kept in
`library.json` beside the path. What crosses the new boundary: one
name, the key.

Next changes: anything new about a video goes in its folder, as today.
Cost: about 70 call sites in one pull request, too large for one.
Worse, the preview renders and the caption files land in Application
Support, which Time Machine backs up and macOS never clears. A preview
render is tens of megabytes, and a person who renders previews of a
hundred clips carries gigabytes of copies in every backup.

### 2. What changes together: what a person made apart from what can be made again

The principle: two kinds of file with two different lives get two
homes. What a person made, the left column above, goes into the
notebook, `Application Support/FrameFairy/videos/<key>/`. What can be
made again, the right column, goes into `~/Library/Caches/FrameFairy/videos/<key>/`,
which macOS and cleaning tools may empty whenever they like, and which
Time Machine leaves out. That is what Apple's own file system guide
asks for: Application Support for what an app has to keep, Caches for
what it can make again.

Layers: the app's library over two folders per video over the files.
What moves: each of today's subfolders is given to one of the two, by
the table above. The `logs` folder splits, because it holds both kinds
today. What crosses: the key, and which of the two folders a file
belongs in, a name for each kind of file.

Next changes: a new kind of file is sorted into one column when it is
added, by asking: does losing it cost the person anything? Cost: the
same 70 call sites as simulation 1, plus the split of `logs`. It keeps
the notebook small, a few megabytes for a long video, most of it the
transcript.

### 3. Who may know about whom: the folder stays, a journal beside the app

The principle: the smallest change, and no part learns anything new.
The work folder stays beside the video. Each time a clip set is written,
the app also writes the clips and their renders into a journal,
`Application Support/FrameFairy/journal/<key>.json`. A video added with
no folder beside it, whose key has a journal, gets its folder rebuilt
from the journal.

Layers: unchanged, plus the journal beside `editPlan`. What crosses:
one call after each plan edit. Next changes: every new field of a clip
has to be written into the journal too, and read back from it. Cost:
small to build. But it is a second copy of every clip, and
ARCHITECTURE.md says why that is a bug waiting to happen: the day the
two disagree, which one is right? A clip fixed in the folder while the
app was closed, by the command line, never reaches the journal. The
transcript is not in the journal, so a rebuilt video has to be heard
again before its clips show words.

### 4. The disk is the truth: the short carries its own story

The principle: the thing made tells where it came from, the way a photo
carries its camera and its time. Each rendered short gets a tag in its
file, written by ffmpeg with `-metadata`: the video's key, the clip's
id, its start and end and its cuts. Picture and sound do not change.
A video added with nothing known about it looks in the shorts folders
for shorts tagged with its key, and brings their clips back as
rendered clips.

Layers: the render writes the tag, the adding of a video reads the
shorts folders. What crosses: the tag's fields. Next changes: none in
the engine, the tag is written once per short. Cost: small. But it
only keeps rendered clips, only while the short is still on the disk,
and a short is usually uploaded and then removed. Clips that were
found but not rendered are lost, and so is the transcript.

### 5. What a test has to build: the engine is handed its folders

The principle: the engine never works out where a video's files are.
It is told. `engine.Machine` gets `Places`, which answers, for a video,
the folder of what it made and the folder of what it can make again.
Each program sets the two roots once when it starts, the way
`Machine.UseSystem` names the caption ink file today. A test hands it
two folders in memory.

Layers: the programs over `Places` over `engine.Episode` and
`engine.Project` over `fileio`. What crosses: `Places.Notebook(video)`
and `Places.Cache(video)`. `WorkDir` and `LogsDir` become those two.
Next changes: where the folders are can change again in one file.
Cost: the 70 call sites move once, in a pull request that changes no
behaviour, because `Places` first answers with the folder beside the
video as today.

## What converges

- **The key.** Simulations 1 to 4 all know a video by its content. The
  fingerprint is already in the code, where the training records use
  it. Reading two megabytes takes a few milliseconds
  on any Mac, reasoned from an SSD's reading speed, not measured. It is
  worked out when a video is added, and again only when the video's
  size or time stamp changed.
- **The app's own folder.** Simulations 1, 2 and 3 keep what a person
  made there. Only simulation 4 does without, and keeps too little.
- **Two homes, not one.** Simulation 2. The notebook stays small and
  is backed up, the caches can go. Simulation 1 would fill backups with
  renders nobody needs.
- **Handed in, not worked out.** Simulation 5, which 1 and 2 need to
  move the 70 call sites without one huge pull request, and which keeps
  the command line on the same notebook as the app.
- **Dropped: the journal,** simulation 3. It is a second copy of every
  clip.
- **Kept for later: the tag in the short,** simulation 4. On its own it
  keeps too little. Beside the notebook it is a safety net for rendered
  clips if the notebook itself is lost. It is cheap to write, so it
  comes as the last step, and only if the user wants it.

## The decision

Simulations 2 and 5 together, with the key. Drawn as layers, each using
only the ones under it:

```
the app: the library (path and key of each video), Remove with Undo,
         Forget in the settings, the watch on the disk
the command line: one video, the same notebook as the app
        │
        ▼
engine.Machine.Places: for a video's key, its notebook and its cache
        │
        ▼
engine.Episode and engine.Project: read and write through Places
        │
        ▼
fileio over afero: the disk, or memory in a test
```

On the disk, for `Folge 12.mp4`:

```
~/Library/Application Support/FrameFairy/
  library.json                 each video's path and key
  videos/3f9a1c7e04b2d8a1/     the notebook
    video.json                 its name, where it was last, its length,
                               when it was added and last opened
    words.json, words.frames   the transcript
    clips-0-1800.json, ...     the clip sets, renders noted in each
    replies/                   the model's answers
    jobs/                      the work that can Continue
    chosen.json, label.json
~/Library/Caches/FrameFairy/videos/3f9a1c7e04b2d8a1/
    probe.json, levels.*, captions/, preview/, prompts and the model's log
~/Movies/Frame Fairy/Folge 12/  the shorts, see the open question
```

Nothing is written beside the video any more. On Windows and Linux the
two roots are the system's own, `%AppData%` and `%LocalAppData%`,
`~/.local/share` and `~/.cache`.

### How it is built, one pull request each

1. **The engine takes a video's folders from `Places`.** It still
   answers with the folder beside the video, so nothing changes for a
   person. Tests hand it memory. About 70 call sites, mostly one line.
2. **The library knows each video by its key.** `library.json` keeps the
   key beside the path. Adding a video whose key is known takes up its
   old entry, whatever its path now.
3. **The notebook and the cache move to the app's folders.** On the
   first start of that build, each video in the list has its folder
   beside it moved over, each file into the home its column says, and
   the emptied folder removed. A video added later with a folder beside
   it is moved the same way. The command line uses the same places.
   The watch now looks at one notebook folder instead of a folder per
   video, and what only a folder beside the video needed goes:
   `added.json`, `deletedOutside`, `clearLetGo` and the guard in
   `MakeDir` against writing back a renamed folder.
4. **Remove without asking, with Undo, and Forget in the settings.**
   The box goes, because removing loses nothing. Forget asks first.
5. **Shorts in a folder of their own.** Shorts already in an `out`
   folder are moved there in step 3's first start, and the clips' notes
   of where their shorts went are written to match.

### What is left for later

- **The tag in the short,** simulation 4, if the user wants a second
  safety net for rendered clips.
- **A video exported again.** The app could offer, in the new video's
  menu, the clips of a removed video with the same name and a length
  within a second of it. Adding them could be undone. Words would be
  matched again from the new transcript.

## What follows

What it buys: a removal, a rename, a move or a deleted file never loses
what a person made. Nothing of the app's sits beside a customer's video,
which was the clutter that made deleting tempting. The watch, the
reconcile and the path checks get simpler, because the app's files are
only in folders the app made itself. A zip with a crafted `.framefairy`
folder beside a video can no longer hand the app plan files, so the
app is safer.

What it costs: five pull requests of a few hundred lines each, and one
move of every existing work folder on the first start of step 3, which
has to be safe against a quit halfway, a full disk and a video whose
folder is on a drive taken out. Adding a video reads two more megabytes
of it.

What would show it was wrong:

- A person works on one video from two Macs, through a shared drive or
  iCloud. Today the folder beside the video travels with it. After this,
  each Mac has its own notebook. If people ask for that, the notebook
  can be exported beside the video on request.
- People re-export their videos often, and expect the clips to follow.
  Then the name-and-length match comes sooner.
- The notebooks grow large. A transcript of four hours is a few
  megabytes, so a hundred videos are well under a gigabyte, reasoned,
  not measured.

## The open question

Where shorts go when no folder is chosen. Two answers fit:

- **`~/Movies/Frame Fairy/Folge 12/`**, recommended. The Movies folder
  is the Mac's own place for a person's videos, Finder shows it in its
  sidebar, and nothing sits beside the customer's video.
- **`Folge 12 Clips/` beside the video.** Closest to what Next to each
  video says today, but it puts a folder of the app's beside the video
  again.
