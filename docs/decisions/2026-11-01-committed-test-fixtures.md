# Committed test fixtures

1 November 2026, for #670.

## The problem

Tim, 1 November 2026: "no test should repeatedly create those fixtures
during test execution. We should have those types of test fixtures
committed into the repository, which not only makes the tests more
reliable, but also makes the tests faster because all of the data is
just already there." And: "maybe we just need a handful of really
different fixtures, and committing it to the repository is just totally
fine. Especially because we are not shipping this with the app."

Measured on main at 6f3bb53, with every ffmpeg call of one run of the Go
tests logged: 83 fixtures made from nothing, 67 of them different, and
about 70 more in the decoder's tests, which skipped there. That is about
160 a run, in 31 test files, 36 s of ffmpeg work when made again one at
a time, 28.8 MB. The walks add 54 videos, kept in CI's cache by their
recipe. Two fixtures are committed today, `engine/testdata/open-gop.mp4`
and `cmd/framefairy-app/testdata/gnome.webm`.

Making them at runtime failed on CI's virtual Mac, where a file is made
on the graphics chip first, #570 and #657, cost minutes of CI, #575 and
#527, and let the app be changed for kinds of file only a test made,
#266, #268, #270, #600, #601, #287 and #553.

The engine's shared test video shows how far they can be shared. Its 69
tests asked for 6 lengths, 2, 6, 15, 20, 40 and 70 seconds. Run with one
length for all, 5 fail, the ones whose length is what they test. Run
with 15, 40 and 70, all pass, at 320 by 180 as well as at 640 by 360.

## Five simulations

### 1. What changes together: a `testdata/` in each package

Go's own convention. Each package keeps the files its tests read, and a
change to one reaches only that package's tests, which
`scripts/changed.sh` already does for `testdata/`.

- Layers: none new.
- What moves: each recipe of a package into a file of its `testdata/`.
- Crossing: nothing, while each package keeps to itself. But the test
  picture with a tone is read by the engine, the app, the walks and
  `internal/outside`. Either it is committed four times, or a package
  reads another's by a path like `../../engine/testdata/`, which no
  import shows.
- Next changes: a new kind of video is added where its first test is,
  and the second package that needs it copies it.
- Cost: 4 folders, the same files in several of them.

### 2. Who may know about whom: one package every test may import

`internal/fixtures` holds every fixture and a `Path` that finds one by
name. It imports nothing of ours, so any test may import it and it can
never pull the app into a test.

- Layers: `internal/fixtures` under the tests of every package, and
  under nothing that ships.
- What moves: every recipe into one table beside the files.
- Crossing: `fixtures.Path(t, name)`, and the names of the files.
- Next changes: a new fixture is a line in the table and a file. A test
  in any package finds it the same way.
- Cost: 1 package, 1 folder. A changed fixture reaches every package
  whose tests import it, through `go list -test`, which is what should
  rerun.

### 3. Where state lives: made once per machine and kept

The walk videos' way since #527, for every test: made the first time,
kept in a folder by the sum of their recipe, cached by CI.

- Layers: a cache under the tests.
- What moves: the recipes stay, a cache goes round them.
- Crossing: the cache's folder, and CI's cache key.
- Next changes: every recipe change makes them all again, on every
  machine.
- Cost: no file committed, but every fresh machine and every change
  still runs ffmpeg, on CI's Mac on the graphics chip. #657 stays. It is
  what Tim ruled out.

### 4. What runs together: made by `make`, before the tests

The fixtures are a build step, the way `make` builds ffmpeg and the
decoder: a target makes each file when it is missing, and the tests read
what it made.

- Layers: a make target under the tests.
- What moves: every recipe into the Makefile or a script it runs.
- Crossing: the folder the target writes, which every package's tests
  read by path.
- Next changes: a new fixture is a rule in the Makefile, so every
  change to one reaches every part of CI, the way the Makefile does.
- Cost: no file committed, but every fresh machine, every cloud session
  and every CI runner without a cache makes them again, CI's Mac on its
  own ffmpeg. It is 3 with `make` as the cache.

### 5. What a test has to build: no video at all

Most of the tests that read the shared test video do not look at it.
`fakeRecognizer` in `engine/project_test.go` hears only how many samples
it is handed, and the fake model answers from the transcript. A search
needs the video's length and some sound, not a file. The engine would be
handed what reads a video, the way the clock, the disk and the models
are handed in, and a test hands in one that gives a length and silence.

- Layers: a reader of media handed to the engine, under it the real one
  over ffmpeg, beside it a test's.
- What moves: every place the engine probes, decodes the sound of or
  frames a video goes through that reader. Counted on the code: the
  engine reaches ffmpeg through 16 methods in `engine/ffmpeg.go`, `Probe`,
  `RunFFmpeg` and `soundOf` among them, called from 11 files of
  `engine/`.
- Crossing: the methods of the reader that a test can answer, `Probe`
  and the sound at least, and what they return.
- Next changes: a test of a search, a window or a comparison needs no
  ffmpeg at all. A test of decoding, framing or the render still needs a
  real file, so the fixtures stay for those.
- Cost: a change to the engine itself, larger than any of the others,
  for tests that today take a second or two each. And the tests would
  no longer run the ffmpeg the app runs, which is what found #570.

Two ways of storing the files of 2 were weighed and are not designs of
their own: Git LFS, which keeps them outside Git's store but needs
`git-lfs` on every machine and returns pointer files without it, and
`go:embed`, which puts every fixture in every test binary that imports
the package, and each test writes it to the disk again for ffmpeg.

## What converges

- Committed files: in 1 and 2, and what 3 and 4 would cache. The
  decisive reason is Tim's rule, and that only committed files keep
  ffmpeg out of every run on every machine.
- One home that every package's tests share: 2, and 4 by its folder. 1
  copies the test picture or reaches across packages by path.
- A plain file on the disk, found by its path: 1 to 4. ffmpeg, the
  decoder and the walks' browser read files.
- The way each was made, kept beside it: 1 to 4. A fixture made again
  on purpose comes out the same, and a reader sees what it is.

Dropped: 3 and 4, which make the files again on every fresh machine,
the cost Tim named. 5 is the better answer for the tests that do not
look at the video, but it changes the engine for tests that are not
slow, and it does not remove the fixtures the others need. It is not
chosen now. If a test of a search ever grows slow for want of it, it is
an issue of its own.

## The decision

```
   tests of engine, cmd/framefairy-app, internal/outside, the walks' bridge
                                   |
                         internal/fixtures   Path, Dir
                                   |
                  internal/fixtures/testdata   the committed files
```

- Every video or sound a test reads is a file in
  `internal/fixtures/testdata/`, read with `fixtures.Path`. A test that
  changes one works on a copy.
- Each is made by its recipe in `internal/fixtures/fixtures_test.go`,
  and `go test ./internal/fixtures -update` makes the ones that are
  missing or whose recipe changed, and leaves the rest as they are, so
  Git keeps no copy made by another ffmpeg. `testdata/recipes.sum` says
  which recipe made each. Without the flag, that test checks the folder
  holds the recipes and nothing else, each made by its recipe as it is.
- `TestNoTestMakesItsOwnFixture` in `internal/ffmpegtest` names a test
  file that asks ffmpeg for `lavfi` while it runs. One file is let off,
  with its reason: `cmd/framefairy-app/cameravideos_test.go`, the
  camera's videos only `speed.mjs` and `smooth.mjs` ask for, which are
  run by hand and never by a test, and a minute of 1080p moving all
  over is too large to commit.
- What only a probe run by hand reads is made when it is asked for, as
  it was: the four hours and the ten minutes `frontend/preview/open.mjs`
  makes for the preview harness, kept in the temp folder. They would add
  49 MB to every clone, for a probe a session runs by hand, and no test
  waits for them.
- As few as the tests allow: a new test reads one that is there, unless
  what it tests is a property none of them has. Then it adds one, as
  small and as short as the test allows.
- `engine/testdata/open-gop.mp4` and `cmd/framefairy-app/testdata/gnome.webm`
  move in as their packages' tests move, `gnome.webm` as
  `recording.webm`, made by a recipe like every other.
- A fixture made from another, a copy of it put together otherwise, names
  it in its recipe as `@` and its name, so it is made first and its sum
  counts in the other's. One written the way a crash leaves it ends in
  `pipe:1`, and what ffmpeg sends there is the file. A test that writes
  beside its file, or changes it, takes a copy with `fixtures.Copy`, or
  with `fixtures.CopyTo` where it cannot end the test, like the walks'
  bridge answering a walk. A walk asks for a video by what it is, and
  the bridge names the fixture, see `walkFixture`, so the walks' cache in
  CI is gone.
- The move was made over three pull requests, in the order of #670's
  Progress list, the engine's tests first and the app's and the walks'
  last.

## What follows

- Costs: the repository grows by the fixtures, and each change to one
  is a new copy in Git's history. With every engine test on them, 78
  files take 24 MB on the disk, and what a clone of main fetches grows
  from 7.2 to 14.2 MB, measured with `git pack-objects` on 2 November
  2026. A fixture is made small before it is committed.
- Buys: no ffmpeg run to make an input, so no input made on CI's
  graphics chip, and the time it took. The tests read the same bytes on
  every machine, so a failure is about the code, not about the ffmpeg
  that made its input.
- Wrong if: the repository grows past what a clone takes in a few
  seconds, or a fixture has to change with nearly every pull request.
  Then the fixtures that change are made by the tests again, and the
  rest stay.
