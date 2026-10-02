# framefairy, the command line

Turns a long-form podcast master into finished vertical clips. You give it
the episode, nothing else. It transcribes the audio, times every word, lets a
language model pick the moments, works out the vertical framing, burns in
captions and writes the clips. By default all of it runs on your machine, with
no API and no cost per run. ffmpeg does the video work.

```
framefairy episode.mp4
```

Install the tools and models first, as [INSTALL.md](INSTALL.md) describes.

## Build

```
make
```

That builds `bin/framefairy` together with the other programs. Copy it anywhere
on your PATH, for instance `sudo cp bin/framefairy /usr/local/bin/`.
[BUILD.md](BUILD.md) covers building without make.

## Run

```
framefairy episode.mp4
```

What happens, in order:

1. ffmpeg and the language model are checked before anything slow starts
2. the audio is transcribed on your machine, every word timed and moved onto
   the sound it belongs to, loudness measured every 10 ms
3. the words are grouped into numbered lines, annotated with pauses and
   loudness
4. the language model picks the moments and answers with the runs of lines
   to keep
5. each clip is scanned for camera switches, and each angle gets one crop
6. captions are built from the same words and burned in
7. one ffmpeg pass per clip produces the finished file

Everything lands next to the source, in `episode.framefairy/`:

```
episode.framefairy/
├── logs/              the record of the run
│   ├── words.json     the transcript, reused on every later run
│   ├── clips.json     the plan
│   ├── proof.txt      what each clip contains, in text
│   ├── reply-*.json   each answer of the model, reused for the same prompt,
│   │                  with how long a local model read, thought and wrote
│   └── ...            prompts, the model's log and token usage
├── captions/          per-clip srt and word timings, plus generated ass
├── preview/           renders from --preview
└── out/               the finished clips and their thumbnails
```

A clip whose plan lists `thumbnails`, moments of the episode in seconds,
gets a picture of the short at each of them, `<name>-1.jpg`, `<name>-2.jpg`
and on, beside `<name>.mp4`. Each is a frame of the finished short, captions
included. Pictures of an earlier render that the plan no longer asks for
are removed. `--preview` and `--dry-run` write none. See
[THUMBNAILS.md](THUMBNAILS.md).

`framefairy episode.mp4 --transcribe-only` transcribes and stops. It writes
`logs/words.srt`, a readable transcript, and makes no API call.

Transcription runs once per episode, or once per `--from`/`--to` window. Its
result is reused until the video file changes.

## Correcting captions

Captions are always made from the episode's words, see
[WORDS.md](WORDS.md), and words are corrected in the app, in the caption
box over the picture. A correction is kept for the episode in
`logs/corrections.json`, and every clip that says the word says it
corrected, from the command line too. Run the same command again, without
`--replan`, and the clips are rendered with the corrected words. Add
`--clip 01` to render only that one.

The caption files the render writes into `captions/` are output, made
afresh every time and never read back. `--refresh-captions` is kept so a
script that passes it still runs, and it does nothing, because there is
nothing left to refresh.

## Flags

**Choosing clips**

| Flag | Default | Effect |
| --- | --- | --- |
| `--count 6` | one for every twelve clip lengths of a half hour window, other windows by the square root of their length, at least 1 | how many clips to look for. 6 for half an hour of clips of 20 to 30 seconds, 3 for 10 minutes, 8 for an hour. The model gives fewer when fewer moments are strong enough |
| `--min 20` | 20 s | shortest acceptable clip |
| `--max 30` | 30 s | target ceiling. A clip that runs a little over is fine |
| `--from 1:00:00 --to 2:00:00` | whole episode | only work on this part of it, see below |
| `--max-pause 2.0` | off | a pause longer than this is always cut, even inside a run the model kept. Meant for recording faults |
| `--keep-pause 0.10` | 0.10 s | air left on each side of a cut |
| `--silence-db -42` | measured | what counts as silence. Taken from the audio when not given |
| `--context ""` | empty | guest name, company, vocabulary. Helps the choice of moments and the spelling of names |
| `--recipe lines` | `lines` | how the model is asked for clips, see [Trying other ways of asking](#trying-other-ways-of-asking) |
| `--compare lines,stories` | off | search the window once with each recipe and write a report, see below. `stories@1024` is `stories` thinking 1024 tokens |
| `--replan` | off | discard the saved plan and choose again |
| `--plan-only` | off | write `clips.json` and stop |
| `--transcribe-only` | off | transcribe, write `logs/words.srt` and stop |
| `--asr-model DIR` | `~/.framefairy/models/...` | where the speech model is |
| `--training-dir DIR` | `~/.framefairy/training` | the one folder the training records of every episode go in, also `FRAMEFAIRY_TRAINING` |

**Rendering**

| Flag | Default | Effect |
| --- | --- | --- |
| `--clip 01` | all clips | render only this clip id, repeatable |
| `--preview` | off | half size, fast preset, into `preview/` |
| `--crf 18` | 18 | quality, lower is better. Every encoder is asked in its own language, so this becomes `-q:v` on Apple's encoder, where higher is better: 18 is 85 there |
| `--preset slow` | slow | x264 speed against compression. Only libx264 has presets, and any other encoder ignores it |
| `--encoder` | the best this ffmpeg has | the video encoder to use. macOS reaches for `h264_videotoolbox` first and falls back to `libx264`, everywhere else it is `libx264` for now |
| `--audio-bitrate 256k` | 256k | aac bitrate |
| `--width 1080 --height 1920` | 1080x1920 | output size in pixels |
| `--no-upscale` | off | write the native crop, no resampling |
| `--no-captions` | off | no burned-in captions. The srt files are still written |
| `--font "Inter Black"` | Inter Black | caption font: Inter Black, Anton and Archivo Black ship inside the program, any other name has to be installed on the machine |
| `--font-size 96` | 96 | caption size, in pixels of a 1080x1920 frame |
| `--margin-v 300` | 300 | caption distance from the bottom edge, same scale |
| `--highlight-colour "#942192"` | purple | colour of the pill behind the word being spoken, for a plan that was not given one in the app |
| `--no-highlight` | off | plain captions, without the bouncing word |
| `--refresh-captions` | off | does nothing: captions are always made from the words |

**Choosing the language model**

| Flag | Default | Effect |
| --- | --- | --- |
| `--planner local` | local | `local` plans on this machine, `api` uses a model in the cloud, see `--model` |
| `--llm-model FILE` | the only `.gguf` in `~/.framefairy/models` | the local model file. A bare file name is also looked for in `~/.framefairy/models` |
| `--llm-server PATH` | `llama-server` on PATH | the llama.cpp server program |
| `--llm-url URL` | none | use a llama-server you already started, which saves loading the model on every run |
| `--model claude-sonnet-5` | claude-sonnet-5 | the model in the cloud, with `--planner api`. A `claude-` model is Anthropic's and reads `ANTHROPIC_API_KEY`, a `gpt-` model, like `gpt-6-sol`, is OpenAI's and reads `OPENAI_API_KEY` |
| `--budget 2.00` | $2.00 | with `--planner api`, refuse a request estimated to cost more than this |
| `--max-tokens 48000` | 48000 | ceiling on the reply length |
| `--think 2048` | 2,048 for half an hour of window, in proportion, at least 512 and at most 4,096 | with `--planner local`, how many tokens the model may think before it answers. `-1` is no limit, `0` is no thinking |
| `--seed N` | 0, and 1 in a comparison | with `--planner local`, makes the model answer the same prompt the same way every time. 0 leaves it to chance |
| `--temperature T` | llama-server's own, 0.8 | with `--planner local`, how freely the model picks its words, 0 always the likeliest |
| `--prefill` | off | with `--planner api`, start the reply with an opening brace |

**Tools and output**

| Flag | Default | Effect |
| --- | --- | --- |
| `--ffmpeg /path/to/ffmpeg` | `ffmpeg` on PATH | use a different build |
| `--ffprobe /path/to/ffprobe` | next to `--ffmpeg` | rarely needed |
| `--out DIR` | `<episode>.framefairy/out` | where finished clips go |
| `--verbose` | off | show every ffmpeg command and API detail |
| `--no-colour` | auto | plain output |
| `--events FILE` | off | also write every step and progress update to FILE as JSON lines, the same feed the app will use |
| `--dry-run` | off | print the ffmpeg commands instead of running them |
| `--version` | | print the version and stop |

Environment variables: `FRAMEFAIRY_FFMPEG`, `FRAMEFAIRY_FFPROBE`, `FRAMEFAIRY_ASR_MODEL`
and `FRAMEFAIRY_TRAINING` set the same as their flags. `FRAMEFAIRY_ASR_PROVIDER=coreml`
tries Apple's CoreML for the recogniser instead of the CPU, which may or may
not be faster on your Mac. `FRAMEFAIRY_NO_FACES=1` turns face detection off.

Options can come before or after the episode, values can be joined with `=`,
and a unique beginning of an option name is enough.

## Working a long episode in passes

`--from` and `--to` restrict a run to one window:

```
framefairy episode.mp4 --from 0       --to 1:00:00
framefairy episode.mp4 --from 1:00:00 --to 2:00:00
```

Only that audio is transcribed and sent, so the model chooses from one hour
rather than four and its attention stays even. Each window gets its own
transcript and plan file, and clip ids start with the window start in seconds
(`t3600-01`), so passes accumulate in `out/`. A later run without `--from`
and `--to` finds a single existing plan on its own, and asks which one to use
when there are several.

A window is sent to the model in one request, and never split behind your
back. So a window longer than the model reads at once is refused before
anything is loaded or paid for, with what the window weighs and what the
model takes, and so is one too short for `--count` clips of `--min` seconds
one after another. How much a model reads is in
[ENGINE.md](ENGINE.md#how-much-one-search-can-read).

## Trying other ways of asking

How the model is asked for clips is a recipe: what it is told, how the
transcript is written out for it, and what its answer looks like. `lines`
is how clips have always been chosen, and the app uses it. `stories` is
the first other one: a brief that fits any video, the transcript as
sentences in paragraphs with a time at the start of each, and "up to 12,
the strongest first" rather than exactly 12. `stories-edit` is `stories`
asked twice, the second time only about where every clip starts and ends,
with the thinking split between the two. What each recipe does is in
[ENGINE.md](ENGINE.md#recipes).

```
framefairy episode.mp4 --from 0 --to 30:00 --compare stories,stories-edit
```

shows whether the second ask makes better edges, and what it costs.
`heart` asks once: the model names the heart of every clip and the
program fits each to the length around it, see
[ENGINE.md](ENGINE.md#recipes).

```
framefairy episode.mp4 --from 0 --to 30:00 --compare lines,heart,heart@1024,heart@0
```

shows whether `heart` finds stories as good as `lines` and how much
sooner, and how much of the thinking it still needs once it no longer
has to count seconds. `heart-opening` asks for the line a clip opens on
too, so a story keeps the setup a stranger needs.

A recipe with `@` and a number thinks that many tokens, whatever
`--think` says, so one recipe can be compared with itself:

```
framefairy episode.mp4 --from 0 --to 30:00 --compare stories,stories@1024
```

Each side has its own folder in `experiments/`, named as it was written.
After a `~` goes the temperature, so `stories@1024~0.3` thinks 1024 tokens
at a temperature of 0.3.

Every side of a comparison draws with the same seed, 1 unless `--seed`
says otherwise. Without it, two runs of the very same prompt came back
with different clips. With it, the same side run again gives the same
answer, so a run can be repeated. Two different sides are still two
different draws, so one window cannot tell a small difference from luck:
it takes the same comparison over many windows, or with several seeds.

```
framefairy episode.mp4 --from 0 --to 30:00 --compare lines,stories
```

searches the same window once with each recipe and writes
`<episode>.framefairy/experiments/compare-<date>.md`: a table of what each
search cost, the time, how many times the model was asked, the seconds it
took over all of them and how many of them went on reading, the tokens it
read and how many of those were new rather than in llama-server's cache
from the side before, the tokens it thought, the size of the request
and what the local model read and wrote, a table of what can be counted
about the clips, how many start or end mid-sentence and how many are well
off the length, and then every clip each found, with its title and the
words that stay, to read side by side. Each recipe's plan is in
`experiments/<recipe>/`, beside `prompt.txt`, what it asked, and
`reply.json`, what came back. So `experiments/` holds everything a
comparison made. When every search failed, there is nothing to
compare: the comparison fails with the reason and writes no report.

A search with any recipe but `lines`, and every search of a comparison, is
an experiment. Its plan goes in `experiments/`, the episode's own plan and
captions are left alone, nothing is rendered, and nothing is recorded for
training, because the training records are answers to one way of asking.
A comparison asks the model afresh each time, since a saved answer costs
nothing and would make the comparison of cost meaningless.

## Not paying twice

Every plan answer, local or from the API, is saved in `logs/` and keyed by a
hash of the prompt and the model. Re-running with the same transcript reuses
it without asking the model again. `--replan` asks again.

With `--planner api`, transient failures are retried up to four times with backoff, honouring
`Retry-After`. Before a request is sent, the tool checks that the window fits
the model's context and `--budget`, and that `--max-tokens` is within the
model's maximum output. A run ends by reporting what it
spent.

## When something goes wrong

Every API call is written to `logs/` before anything reads it. Every step is
timestamped, slow steps show progress, and `--verbose` adds the exact ffmpeg
command for every pass. A finished clip whose length differs from the plan by
more than 0.5 s is reported as a failure.

A plan made by an earlier version has no word timings. Its clips still render
with any per-clip srt files you already have, and a clip without one renders
without captions and says so. `--replan` makes a new plan with words.

## Input handling

The clip plan and everything the model returns are treated as untrusted.

- clip ids and slugs are restricted to letters, digits, hyphen and underscore
- every output path is verified to stay inside its folder
- times must be finite and non-negative, at most 300 segments per clip
- ASS override syntax and control characters are removed from caption text
- `caption_style` values are validated by type and colours must look like
  colours
- two clips resolving to the same file name are an error
- the API key must be a valid header value, and redirects are never followed

## What it touches

- **Processes:** `ffmpeg`, `ffprobe`, `llama-server`, and on macOS `security`
  for the keychain.
  Always as argument lists, never through a shell.
- **Files:** everything goes in `<episode>.framefairy/` next to the source, apart
  from `--out` if given, plus short-lived temp files. The speech model is only
  read.
- **Network:** none with the default local planner. The tool talks to its own
  llama-server on 127.0.0.1 only. With `--planner api`, one endpoint,
  `https://api.anthropic.com/v1/messages`.
- **Deletion:** nothing you wrote. A clip whose render failed or was
  interrupted is removed, and so are generated `.ass` files, which are rebuilt
  on every render.
- **Interrupts:** ctrl-c is safe at any point.

With the local planner nothing leaves your machine. With `--planner api`, the
transcript text is sent to the API once per plan. The audio never leaves your
machine.
