# Training the clip selection model

This extends `framefairy-training-plan.md` with what is now built and decided.
Everything here lives in the engine and in `framefairy-train`. The app records
what happens to clips and shows nothing about training.

---

## 1. Answer format

The model answers with line ranges. Each pair `[first, last]` is a run of
consecutive lines to keep.

### Version 2 (current)

- A pause between two lines inside one run stays, at full length.
- Two runs may follow each other directly. `[[12, 14], [15, 18]]` keeps lines
  12 to 18 and cuts only the pause between 14 and 15.
- Leaving lines out drops them, together with the pauses around them.
- Pairs are ascending and never overlap.

Every pause of at least 0.45 seconds ends a line (or the `--max-pause` value,
when it is set). So every pause long enough to matter lies between two lines,
and version 2 can express cutting or keeping each of them.

A cut leaves 0.1 seconds (`--keep-pause`) of silence after the last word and
before the next one. Where a pause is shorter than that, the two runs join
into one piece and nothing plays twice.

### Version 1

Runs could not follow each other directly. A pause could only be cut by
dropping a line. Version 2 only adds to version 1, so every version 1 answer
is still a valid version 2 answer.

`PromptVersion` in `engine/training.go` changes whenever the prompt or the
line rules change. It is stored in every plan record.

---

## 2. What is recorded

One folder for every episode, `~/.framefairy/training` unless `FRAMEFAIRY_TRAINING`,
`--training-dir` or the app's setting says otherwise. Append only, one JSON
record per line, in `plans.jsonl` and `decisions.jsonl`.

The records do not live with an episode. Letting go of a video takes
everything the app made of it, work folder and all, and the records outlive
that: they are examples of what a model was asked and what a person did with
the answer, and they are still true when the video is gone. Every plan record
carries the key of the episode it came from, so one folder holds them all
without mixing them up. The app's settings screen says where the folder is,
how many records are in it, and throws them away when asked.

### `plans.jsonl`

One record per model answer. Next to the fields from the original plan:

| Field | Meaning |
| --- | --- |
| `example_key` | hash of the user prompt, which is the transcript and the request |
| `episode.key` | fingerprint of the video from its size and its first and last megabyte |
| `lines` | the word timings of every numbered line the model saw |
| `candidates[].segments` | each proposed clip as it was first made, before any change |

### `decisions.jsonl`

One record per action on a clip.

| Event | Recorded when |
| --- | --- |
| `viewed` | a clip is played in the app |
| `edited` | an edge is trimmed, or a cut inside a clip is made, moved or thrown away, in the app |
| `rendered` | a clip finished rendering in the app |
| `rejected` | a clip is removed from the list in the app, or `framefairy-train mark` |
| `kept` | a removed clip is put back in the app, or `framefairy-train mark` |
| `published`, `unpublished` | `framefairy-train mark` |

The app has no keep or reject buttons. Rendering is what a person does with
a good clip, so a render is the positive signal, and removing a clip from
the list is what a person does with one they do not want, so it is the
negative. Neither asks for a reason. A reason is something `framefairy-train
mark` adds later, away from the work.

What is not recorded, on purpose: word corrections, the crop, where the
caption line sits and which font the captions use. They say how a short is
dressed, not whether the model picked a good moment, and the selection model
is the only thing these records train. Mixing them in would teach it that a
clip whose captions were nudged is a better clip. If a model for framing or
for caption placement is ever worth having, it gets records of its own, with
the picture it was deciding about.

Every record except `viewed`, `rejected` and `unpublished` carries:

- `final.segments`, the clip as it is at that moment
- `final.lines`, the same clip in the answer format, with runs that follow
  each other directly wherever a pause is cut
- `changes`, the clip compared with its proposal:

| Field | Meaning |
| --- | --- |
| `unchanged` | edges within 0.05 seconds, and the same cuts in the same places |
| `start_shift`, `end_shift` | seconds the edges moved, negative is earlier |
| `lines_added`, `lines_removed` | line numbers |
| `pauses_cut` | cuts that are new, as `[from, to]` in seconds |
| `pauses_restored` | cuts of the proposal that are gone |
| `pauses_moved` | cuts of the proposal that were kept but put elsewhere, as `{was, now}` |

The pause fields compare a clip with its proposal, and they are the whole
record of what a person thought of the model's cutting. Where a clip is cut
is a judgement the model makes, so every deviation is a lesson:

- **A cut the model did not make.** The person heard something that had to
  go where the model let the audio run. `pauses_cut`.
- **A cut of the model's thrown away.** The model took out something that
  carried the moment. `pauses_restored`.
- **A cut of the model's moved.** The model was right that something
  belonged there and wrong about where it fell. `pauses_moved`, which
  carries both places so the pair can be learned from.

Two cuts that overlap are the same cut, so a cut that was moved is one
`pauses_moved` and never a `pauses_cut` plus a `pauses_restored`. Those are
different lessons and counting one as the other teaches the wrong thing.
A cut is only compared where the proposal and the final clip overlap, so a
trimmed edge is not also counted as a pause change.

`unchanged` is what the export reads to decide whether a render is a full
hit, so it is false whenever any of the three lists is not empty. A cut
moved by less than the 0.05 second tolerance is the same cut, because a
hand is no steadier on the edge of a cut than on the edge of a clip.

---

## 3. Never counting a signal twice

- **Same answer, same prompt:** a new plan record is only written when the
  answer differs. Starting an episode over and getting the same clips again
  reuses the first record and its `plan_id`.
- **Same prompt, different answers:** the export treats all plans with the
  same `example_key` as one example.
- **Same clip, proposed again:** within an example, a clip is identified by
  its proposed lines. All decisions on it, from any plan, update one state,
  and only the latest state counts.
- **Same target twice:** two clips that end up with the same final lines are
  one target.
- **Same video twice:** the episode fingerprint puts copies and renamed files
  into the same split, and their records collapse like any other.

---

## 4. From decisions to training records

The latest state of each proposed clip decides what it becomes, strongest
first:

| State | Outcome | Weight | Target lines |
| --- | --- | --- | --- |
| published | positive | 3 | final |
| rendered | positive, a full hit when unchanged | 2 | final |
| kept or edited | positive | 1 | final |
| rejected | negative | | proposed |
| played, then left | weak negative, see below | | proposed |
| nothing | not used | | |

A clip that was played but never rendered, edited, kept or published counts
as passed over, but only when another clip of the same example was rendered
or published. Then the person was clearly choosing, and this one was not
chosen. A clip nobody played says nothing and is not used.

An edit after a render clears the render, because the rendered version is no
longer the one on record.

### `sft.jsonl`

One record per example with at least one positive. The answer holds every
positive with its final lines.

### `dpo.jsonl`

- **Selection pair:** all positives over all negatives, when both exist.
  When every negative is only passed over, the pair's weight is at most 0.5.
- **Correction pair:** for every changed clip whose final lines differ from
  its proposal, the final clip over the proposed one. Trimmed edges, cut
  pauses and restored pauses all produce this pair.

### Meta data

Every record has a `meta` field with the example key, the weight, the
outcome of each clip, and its `changes`. Training scripts may use the
weights or ignore them.

### Older records

Records made with an older answer format are exported with the current
system prompt, since each version only extends the one before. Their meta
data says `system_upgraded: true` and keeps `recorded_prompt_version`.
Records made before the proposal segments were stored have no `changes`,
and their edits count as plain keeps.

---

## 5. The training tool

`framefairy-train` works with the records. It is not part of the app and is not
shipped with it.

`make` builds it into `bin/framefairy-train` together with the other programs.

### `mark`

```
framefairy-train mark EPISODE [--plan FILE] [--keep IDS] [--reject IDS] [--reason ID=REASON,...] [--published IDS] [--unpublished IDS]
```

Records decisions on the clips of a plan. `--plan` is only needed when the
episode has more than one clip set. IDS are clip ids separated by commas.
Reasons are `no-payoff`, `weak-opening`, `needs-context`, `too-long`,
`too-short`, `bad-cut`, `boring` and `other`. A reason outside that list is
refused before anything is written, because a record that cannot be stored
is a signal lost.

```
framefairy-train mark episode.mp4 --keep 01,04 --reject 02,07 --reason 02=no-payoff --published 04
```

### `export`

```
framefairy-train export --out DIR [--root DIR] [--eval-share 0.2]
```

Reads the one training folder, or `--root` when it is given, and writes the
dataset to `--out`: `train/` and `eval/`, each with `sft.jsonl` and
`dpo.jsonl`, and `manifest.json` with the counts. `--eval-share` is the share
of episodes held out for evaluation. Episodes are split whole, never clip by
clip, and the split never changes between exports. `--root` also finds the
records that older versions kept beside each episode, in
`<episode>.framefairy/training/`, so nothing recorded then is lost.

---

## 6. Open

| # | Item |
| --- | --- |
| T.6 | `framefairy-train import` for clips already published |
| T.7 | `framefairy-train eval` with hit rate on held-out episodes. A candidate counts as a hit when it overlaps a positive by at least half of that clip's length |
| T.8 | `--llm-lora` passed to llama-server and recorded as the adapter |
| T.9 | Lengthening or shortening a pause without cutting it. The answer format has no way to say this, so it would stay meta data |
| T.10 | A separate training app, only if the command line turns out to be too little |
