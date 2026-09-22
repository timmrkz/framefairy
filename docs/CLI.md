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
│   └── ...            prompts, raw replies and token usage
├── captions/          per-clip srt and word timings, plus generated ass
├── preview/           renders from --preview
└── out/               the finished clips and nothing else
```

`framefairy episode.mp4 --transcribe-only` transcribes and stops. It writes
`logs/words.srt`, a readable transcript, and makes no API call.

Transcription runs once per episode, or once per `--from`/`--to` window. Its
result is reused until the video file changes.

## Correcting captions

Edit the per-clip file in `captions/`, for instance `01_werkstatt.srt`. It is
timed to the cut clip, so change the text and leave the timestamps alone. Then
run the same command again, without `--replan` and with the same `--from` and
`--to` if you used them. The saved plan is reused, no API call is made, and
the clips are rendered again with your text. Add `--clip 01` to render only
that one.

The app corrects words in the plan instead. Any change the app makes to a
clip, a trim or a corrected word, removes that clip's caption
file, so the next render builds it from the plan again.

`--refresh-captions` throws your edits away and rebuilds the files from the
words in the plan. `--replan` makes a new plan with new cuts and moves the old
caption files into a `superseded-…` folder, because their timing no longer
fits.

## Flags

**Choosing clips**

| Flag | Default | Effect |
| --- | --- | --- |
| `--count 12` | 12 | how many clips to look for. Candidates cost little extra, so the default asks for plenty to choose from |
| `--min 20` | 20 s | shortest acceptable clip |
| `--max 30` | 30 s | target ceiling. A clip that runs a little over is fine |
| `--from 1:00:00 --to 2:00:00` | whole episode | only work on this part of it, see below |
| `--max-pause 2.0` | off | a pause longer than this is always cut, even inside a run the model kept. Meant for recording faults |
| `--keep-pause 0.10` | 0.10 s | air left on each side of a cut |
| `--silence-db -42` | measured | what counts as silence. Taken from the audio when not given |
| `--context ""` | empty | guest name, company, vocabulary. Helps the choice of moments and the spelling of names |
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
| `--crf 18` | 18 | quality, lower is better. Every encoder is asked in its own language, so this becomes `-q:v` on Apple's encoder, where higher is better |
| `--preset slow` | slow | x264 speed against compression. Only libx264 has presets, and any other encoder ignores it |
| `--encoder` | the best this ffmpeg has | the video encoder to use. macOS reaches for `h264_videotoolbox` first and falls back to `libx264`, everywhere else it is `libx264` for now |
| `--audio-bitrate 256k` | 256k | aac bitrate |
| `--width 1080 --height 1920` | 1080x1920 | output size in pixels |
| `--no-upscale` | off | write the native crop, no resampling |
| `--no-captions` | off | no burned-in captions. The srt files are still written |
| `--font "Inter Black"` | Inter Black | caption font: Inter Black, Anton and Archivo Black ship inside the program, any other name has to be installed on the machine |
| `--font-size 96` | 96 | caption size, in pixels of a 1080x1920 frame |
| `--margin-v 300` | 300 | caption distance from the bottom edge, same scale |
| `--highlight-colour "#942192"` | purple | colour of the pill behind the word being spoken |
| `--no-highlight` | off | plain captions, without the bouncing word |
| `--refresh-captions` | off | rebuild per-clip captions, discarding manual corrections |

**Choosing the language model**

| Flag | Default | Effect |
| --- | --- | --- |
| `--planner local` | local | `local` plans on this machine, `api` uses the Claude API |
| `--llm-model FILE` | the only `.gguf` in `~/.framefairy/models` | the local model file |
| `--llm-server PATH` | `llama-server` on PATH | the llama.cpp server program |
| `--llm-url URL` | none | use a llama-server you already started, which saves loading the model on every run |
| `--model claude-sonnet-5` | claude-sonnet-5 | the Claude model, with `--planner api` |
| `--budget 2.00` | $2.00 | with `--planner api`, refuse a request estimated to cost more than this |
| `--max-tokens 48000` | 48000 | ceiling on the reply length |
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

`--from` and `--to` restrict a run to one stretch:

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

## Not paying twice

Every plan answer, local or from the API, is saved in `logs/` and keyed by a
hash of the prompt and the model. Re-running with the same transcript reuses
it without asking the model again. `--replan` asks again.

With `--planner api`, transient failures are retried up to four times with backoff, honouring
`Retry-After`. Before a request is sent, the tool checks the model's context
window, its maximum output and `--budget`. A run ends by reporting what it
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
