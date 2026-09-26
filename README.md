# Frame Fairy

Turns a long-form podcast episode into finished vertical clips. It
transcribes the audio, times every word, lets a language model pick the
moments, works out the vertical framing, burns in captions and renders the
clips. By default all of it runs on your machine, with no API and no cost per
run.

## The parts

| Part | Folder | What it is | Docs |
| --- | --- | --- | --- |
| `framefairy` | `cmd/framefairy/` | the command line | [docs/CLI.md](docs/CLI.md) |
| `framefairy-app` | `cmd/framefairy-app/`, `frontend/` | the desktop app, Go side and interface | [docs/APP.md](docs/APP.md) |
| engine | `engine/`, `asr/` | the code that does the work, shared by both | [docs/ENGINE.md](docs/ENGINE.md) |
| `framefairy-train` | `cmd/framefairy-train/`, `train/` | turns recorded decisions into training data. Not part of the app | [docs/TRAINING.md](docs/TRAINING.md) |

The command line and the app are two front ends for the same engine. What one
does to an episode, the other can pick up.

## Working on it

Changes arrive as pull requests from Claude's cloud sessions. After a merge:

```
git pull
make run
```

[docs/WORKFLOW.md](docs/WORKFLOW.md) sets this up, once.

## Getting started

```
make run       # installs what is missing, builds everything, starts the app
```

That is the whole of it. The first run takes a while, because it installs
the tools this machine lacks and builds the two programs framefairy ships
beside itself, ffmpeg and llama-server. After that it is an ordinary build. The app then walks you through the rest: it
fetches the speech model itself and asks once how clips should be found.

`make check` lists what the machine still needs. [docs/BUILD.md](docs/BUILD.md)
explains every target, and [docs/INSTALL.md](docs/INSTALL.md) has the manual
steps for every system.

After building:

```
bin/framefairy episode.mp4
bin/framefairy-app
```

## The repository

```
README.md             this page
CLAUDE.md             rules and context for Claude
Makefile              builds everything, see docs/BUILD.md
.github/workflows/    CI for every pull request
go.mod, go.sum        one Go module for everything, Go 1.27
scripts/              the checks, tool installs and model downloads make runs
cmd/
  framefairy/             the command line
  framefairy-app/         the desktop app, Go side
    dist/             the built interface, made by make, not in git
  framefairy-train/       the training tool
  framefairy-release/     makes the update key, signs builds to update to
engine/               transcription, planning, framing, captions, rendering
asr/                  the speech recogniser, the only native code
train/                the dataset export for framefairy-train
updates/              the channel list the app updates itself from
frontend/             the app's interface, Svelte and TypeScript
bin/                  the built programs, made by make
docs/
  BUILD.md            building with make, and without
  WORKFLOW.md         working with Claude through GitHub
  INSTALL.md          tools and models, for every part
  CLI.md              the command line: running it, all flags
  APP.md              the app: using it, how it is built
  ENGINE.md           how the engine works
  TRAINING.md         training records and the training tool
  GUI-PLAN.md         the app's build plan, batch by batch
  ROBUSTNESS.md       what can go wrong between the parts, and what was done
  PACKAGING.md        what ships, what the user installs, which ffmpeg
  UPDATES.md          how the app updates itself, to the next release and,
                      while it is being made, to any pull request
  THUMBNAILS.md       the thumbnail of a short: spec, not built yet
  THIRD_PARTY.md      licences of everything included
```

Go code lives in `cmd/`, `engine/`, `asr/`, `train/` and `updates/`. JavaScript and
TypeScript live only in `frontend/`. The one meeting point is
`cmd/framefairy-app/dist/app/`, which make builds from `frontend/` and the app
embeds, because Go can only embed files from its own folder. Build output,
the built interface included, never goes into git.

## Tests

```
make test
```

The tests need no model and no network. The ones that render are skipped
when ffmpeg is missing.

## Licences

The third-party work used by every part, and what has to ship with a build,
is listed in [docs/THIRD_PARTY.md](docs/THIRD_PARTY.md).
