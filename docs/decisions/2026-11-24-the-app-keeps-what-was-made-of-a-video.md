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
  the folder out from the video's file name. In code that is not a test,
  39 lines call it, 24 call `LogsDir`, which is built on it, and 12 more
  build the `logs` folder by hand, in 25 files. Tests use them 169
  times in 50 files, and 3 walk files do too.
- **Remove** opens a box with three buttons: Cancel, Keep and a red
  Remove. The red Remove deletes the whole folder, `Machine.DeleteWork`
  in `engine/frames.go`: the transcript, every clip set, every word that
  was fixed, and the renders.
- In the user's logs from 7 to 21 November there are 14 removals. Every
  one deleted the folder, `work_deleted: true`. Keep was never chosen.
  8 of the 14 were followed by adding the same video again within 20
  seconds, so the red Remove is also how the user starts a video over.
  The other 6 lost what was made of the video for good.
- **Save clips to** says Next to each video. The app then renders into
  the `out` folder inside that same folder, `shortsFolder` in
  `engine/run.go`. The user's setting is that one. So the red Remove
  deleted their rendered shorts too, not only the record of them.
- A video renamed or moved in Finder is looked for under a new folder
  name, finds none, and starts from nothing. Its old folder stays behind
  where nobody looks.
- A video exported again over the same file keeps its folder today and
  says Changed since transcription, `TranscriptStale`.
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

The shorts in `out/` are in neither column. They are the person's own
files, and the app never deletes them.

## What a person sees once it is built

1. **Remove a video.** It leaves the list at once and its row offers
   itself back for a moment, the way every removed row does. No box
   asks, because nothing is lost. Add it again, tomorrow or in a month,
   and the transcript, the clips and the rendered marks are all there.
2. **Start a video over.** The video's menu has **Start over**. It
   throws away the transcript and the clips and cannot be taken back, so
   it asks first, in `Confirm.svelte`. The shorts stay where they are.
   This is what most of the user's removals were for.
3. **Rename or move the video in Finder,** then add it from its new
   place. Everything is there.
4. **Delete the video, and the shorts,** and download the same file
   again later. Add it, and everything is there, the rendered clips
   marked as rendered.
5. **Export a video again over the same file** while it is in the list.
   It keeps its clips and says Changed since transcription, as today.
6. **Export a video again as a new file** and add it. It is a different
   file, so it starts from nothing. A later step can offer the clips of
   a removed video with the same name and length, see What is left for
   later.
7. **Free the space.** The settings list the removed videos the app
   still remembers, with what each takes on disk, and **Forget**. Forget
   cannot be taken back, so it asks first. It and Start over are the
   only places the app throws away what a person made.
8. **Shorts** go into a folder of their own that a person can see. The
   app never deletes a short.

## Five simulations

### 1. Where state lives: the whole work folder moves to a folder the app owns

The principle: what belongs to a video is kept where the app keeps its
own things, not beside a file it does not own. Every work folder moves
as it is to `~/.framefairy/videos/<key>/`, and `<key>` is the
fingerprint, sixteen letters and digits like `3f9a1c7e04b2d8a1`.
`~/.framefairy/` is where the engine already keeps what it writes for
both programs, the models and the training records.

Layers: the library over `WorkDir(key)` over the files. What moves:
every caller of `WorkDir` and `LogsDir` takes the key instead of the
video's path. What crosses the new boundary: one name, the key.

Next changes: anything new about a video goes in its folder, as today.
Cost: the preview renders and caption files land in a folder that Time
Machine backs up and nothing ever clears. A preview render is tens of
megabytes, and a hundred of them are gigabytes in every backup.

### 2. What changes together: what a person made apart from what can be made again

The principle: two kinds of file with two different lives get two
homes. What a person made, the left column above, goes into the
notebook, `~/.framefairy/videos/<key>/`. What can be made again, the
right column, goes into the system's cache folder,
`~/Library/Caches/FrameFairy/videos/<key>/` on a Mac, which macOS and
cleaning tools may empty whenever they like, and which Time Machine
leaves out. That is what Apple's own file system guide asks for.

Layers: the library over two folders per video over the files. What
moves: each of today's subfolders goes to one of the two, by the table
above. The `logs` folder splits, because it holds both kinds today.
What crosses: the key, and which of the two folders a kind of file
belongs in.

Next changes: a new kind of file is sorted into one column when it is
added, by asking: does losing it cost the person anything? Cost: the
same callers as simulation 1, plus the split of `logs`. The notebook
stays a few megabytes for a long video, most of it the transcript.

### 3. Who may know about whom: the folder stays, a journal beside it

The principle: the smallest change, and no part learns anything new.
The work folder stays beside the video. Each time a clip set is written,
the app also writes the clips and their renders into a journal,
`~/.framefairy/journal/<key>.json`. A video added with no folder beside
it, whose key has a journal, gets its folder rebuilt from the journal.

Layers: unchanged, plus the journal beside `editPlan`. What crosses:
one call after each plan edit. Next changes: every new field of a clip
has to be written into the journal too, and read back from it. Cost:
small to build. But it is a second copy of every clip, and
ARCHITECTURE.md says why that is a bug waiting to happen. A clip
changed by the command line never reaches the journal. The transcript
is not in it, so a rebuilt video has to be heard again.

### 4. The disk is the truth: the short carries its own story

The principle: the thing made tells where it came from, the way a photo
carries its camera and its time. Each rendered short gets a tag in its
file, written by ffmpeg with `-metadata`: the video's key, the clip's
id, its start and end and its cuts. Picture and sound do not change.
A video added with nothing known about it looks in the shorts folders
for shorts tagged with its key, and brings their clips back as
rendered clips.

Layers: the render writes the tag, adding a video reads the shorts
folders. What crosses: the tag's fields. Cost: small. But it only keeps
rendered clips, only while the short is still on the disk, and a short
is usually uploaded and then removed. Clips that were found but not
rendered are lost, and so is the transcript.

### 5. What runs together and what a test has to build: the engine is handed its folders

The principle: the engine never works out where a video's files are.
It is told, and both programs and every test ask the same one thing.
`engine.Machine` gets `Places`. Given a video's path, it answers the
folder of what was made and the folder of what can be made again. Each
program sets the roots once when it starts, the way `Machine.UseSystem`
names the caption ink file today. A test hands it folders in memory.

What runs together: the app and the command line both run on one video
at times, so both have to find the same notebook, and the one file that
links a path to a key is written by both. It is written whole and
atomically through `fileio`, as every file is, and the app's watch
hears the command line's change. Nothing moves a folder while a job
writes in it, because the one move, see below, runs before any job
starts.

Layers: the programs over `Places` over `engine.Episode` and
`engine.Project` over `fileio`. What crosses: `Places.Notebook(video)`
and `Places.Cache(video)`. `WorkDir` and `LogsDir` become those two.
Next changes: where the folders are can change again in one file.
Cost: the callers move once, in pull requests that change no
behaviour, because `Places` first answers with the folder beside the
video as today.

## What converges

- **The key.** Simulations 1 to 4 all know a video by its content. The
  fingerprint is already in the code, where the training records use
  it. Reading two megabytes takes a few milliseconds on an SSD, reasoned,
  not measured. For a video in iCloud that is not downloaded, it waits
  for the download, as opening the video does today.
- **A folder the app owns.** Simulations 1, 2 and 3. Only simulation 4
  does without, and keeps too little.
- **Two homes, not one.** Simulation 2. The notebook stays small and is
  backed up, the caches can go. Simulation 1 would fill backups with
  renders nobody needs.
- **Handed in, not worked out.** Simulation 5, which 1 and 2 need to
  move the callers without one huge pull request, and which keeps the
  command line on the same notebook as the app.
- **Dropped: the journal,** simulation 3. It is a second copy of every
  clip.
- **Kept for later: the tag in the short,** simulation 4. On its own it
  keeps too little. Beside the notebook it is a safety net for rendered
  clips if the notebook itself is lost. Only if the user wants it.

## The decision

Simulations 2 and 5 together, with the key. Drawn as layers, each using
only the ones under it:

```
the app: the library (paths only), Remove with Undo, Start over,
         Forget in the settings, the watch on the disk
the command line and the training tool: the same Places
        │
        ▼
engine.Machine.Places: a video's path to its key, through one index,
        and the key to its notebook and its cache
        │
        ▼
engine.Episode and engine.Project: read and write through Places
        │
        ▼
fileio over afero: the disk, or memory in a test
```

On the disk, for `Folge 12.mp4`:

```
~/.framefairy/videos/
  index.json                   each path to its key, with the video's
                               size and time, the one home of the link
  3f9a1c7e04b2d8a1/            the notebook
    video.json                 its name, its length, every key its
                               content has had, when it was added and
                               last opened
    words.json, words.frames   the transcript
    clips-0-1800.json, ...     the clip sets, renders noted in each
    replies/                   the model's answers
    jobs/                      the work that can Continue
    chosen.json, label.json
~/Library/Caches/FrameFairy/videos/3f9a1c7e04b2d8a1/
    probe.json, levels.*, captions/, preview/, prompts and the model's log
~/Movies/Frame Fairy/Folge 12/  the shorts, see the open question
```

Nothing is written beside the video any more. `library.json` stays a
list of paths, as today.

### The rules that hold it together

- **One home for the link.** `index.json` alone links a path to a key.
  `video.json` keeps no path. The key of a video in the library is held
  on its `Episode` by the file's stamp, so a read of a transcript or a
  file served asks memory, not the disk.
- **A key is worked out only when a video is added.** A video in the
  list whose content changes keeps its notebook, and the new
  fingerprint joins the keys in its `video.json`, so adding the changed
  file later finds the notebook too. It says Changed since
  transcription, as today.
- **A key is checked before it is a path.** An empty fingerprint, which
  `EpisodeKey` gives for a file it cannot open, is no key, and the
  video is refused as it would be today. A key from `index.json` is
  taken only as sixteen lowercase hex characters, and the folder is
  made through `SafeChild`, because Forget and Start over delete by it.
- **The length is checked too.** `video.json` keeps the length. A key
  whose notebook has another length is a different video that happens
  to share size, start and end, and it gets a notebook of its own.
- **Two copies of one file** have one key. While one is in the list,
  adding the other is refused and says why, as `AddEpisodes` refuses
  two videos that would share a folder today. Once it is removed, the
  copy takes up the notebook.

### How it is built, one pull request each

1. **Stop the loss, on today's layout.** Remove takes the video off the
   list and keeps its folder, with the row offering itself back, and the
   box goes. **Start over** in the video's menu asks first and deletes
   the folder, as the red Remove does today. Shorts with no folder
   chosen go to `~/Movies/Frame Fairy/<video>/` instead of `out/`.
   Shorts already rendered stay where they are, and their clips know
   where. This alone fixes what the user reported for every video whose
   name and place stay the same.
2. **The engine takes a video's folders from `Places`.** It still
   answers with the folder beside the video, so nothing changes for a
   person. The engine's callers first.
3. **The app, the command line and the training tool take theirs from
   `Places` too,** with the tests. Still nothing changes for a person.
4. **The keys and the notebook.** `index.json`, the checks above, the
   notebook and the cache in their new homes. On the first start of that
   build, each video in the list has the left column of its folder
   moved into its notebook, its caches are dropped and made again when
   asked for, and the emptied folder is removed. `out/` and anything the
   app did not write stays where it is. The move reads only small files,
   so it takes seconds, and it runs before any job starts, off the
   window's path. A video added later with a `.framefairy` folder beside
   it is moved the same way, but only through the same checks as any
   file from outside: no link is followed, `Linked`, only regular files
   are taken, and each is read by its own reader, `ReadPlan`,
   `LoadClips` and the transcript's, before it is written into the
   notebook. A file that fails stays behind. The watch then looks at
   `~/.framefairy/videos` instead of a folder per video, and what only a
   folder beside the video needed goes: `added.json`, `deletedOutside`,
   `clearLetGo` and the guard in `MakeDir`. The command line's docs
   change with it: what it writes for a person to read, the comparison
   reports and `proof.txt`, it writes into the folder it was run in,
   and it prints where.
5. **Forget in the settings,** with the list of removed videos and what
   each takes on disk.

### What changes in ARCHITECTURE.md, and which records it replaces

Step 4 rewrites, in `docs/ARCHITECTURE.md`: the first row of Where
state lives, a video's folder with Finder as a writer, and the rows of
the probe and the loudness, which move to the cache. The fifth
invariant, the files on the disk as the truth about a video, which
stays true of the notebook. The line under Boundaries on the files of a
work folder. And the rows of Where does it go for reading a file of a
video, `MakeDir`, and noticing a change outside the app, `reconcile`.
Step 4 also adds a Replaced by line, in part, to
[the app in step with its files](2026-11-01-the-app-in-step-with-its-files.md),
where the work folder beside the video was the truth, and to
[a video's probe kept in its work folder](2026-11-19-a-videos-probe-kept-in-its-work-folder.md).

### On Windows and Linux

The notebook is in `~/.framefairy/videos/` on all three, in the home
folder and never in a profile that roams. The cache is the system's,
`os.UserCacheDir`: `%LocalAppData%` on Windows, `~/.cache` on Linux.
Shorts go to the system's folder for videos, Videos on Windows and
`~/Videos` on Linux, because neither has a Movies folder.

### What is left for later

- **The tag in the short,** simulation 4, if the user wants a second
  safety net for rendered clips.
- **A video exported again as a new file.** The app could offer, in the
  new video's menu, the clips of a removed video with the same name and
  a length within a second of it. Adding them could be undone.

## What follows

What it buys: a removal, a rename, a move or a deleted file never loses
what a person made, and starting over is one step that asks first.
Nothing of the app's sits beside a customer's video. The watch, the
reconcile and the path checks get simpler, because once the first
start has moved them, the app's files are only in folders the app made
itself.

Is it safer? Once a video's folder has been moved, yes: nothing beside a
video is read or written. A folder found beside a video that is added
is still read once, and that import takes nothing it would not take
from any file from outside, see step 4.

What it costs: five pull requests, step 1 small and steps 2 to 4 each a
few hundred lines, and one move of every work folder on the first start
of step 4. Adding a video reads two more megabytes of it.

What it breaks: a build from before step 4, picked from Updates after
step 4 has run, finds no folder beside the videos and lets them go from
its list. Nothing is lost, because the notebooks stay where they are,
and the newer build finds them again when the videos are added. A
folder the older build made beside a video in the meantime is left
alone and logged when the newer build finds a notebook for its key
already there.

What would show it was wrong:

- A person works on one video from two Macs, through a shared drive or
  iCloud. Today the folder beside the video travels with it. After this,
  each Mac has its own notebook. If people ask for that, the notebook
  can be exported beside the video on request.
- People export their videos again as new files often, and expect the
  clips to follow. Then the name-and-length match comes sooner.
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
