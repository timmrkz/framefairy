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

### 4. What runs together: Git LFS

The files are committed, but kept outside Git's own store.

- Layers: none new in the code. A step in every checkout.
- What moves: the files into LFS.
- Crossing: `.gitattributes`, and `git lfs` in CI, in the cloud
  sessions' setup and on Tim's Mac.
- Next changes: none in the code.
- Cost: every machine needs `git-lfs`, GitHub counts LFS bandwidth, and
  a checkout without it gets pointer files that fail as broken videos.
  For files of a few MB it buys nothing.

### 5. What a test has to build: fixtures embedded with `go:embed`

A package embeds the files, and a test gets their bytes.

- Layers: as in 2.
- What moves: as in 2, and every test writes the bytes to a file again,
  because ffmpeg and the decoder read a path.
- Crossing: a `[]byte` or an `fs.FS`.
- Next changes: as in 2.
- Cost: every test binary that imports it carries every fixture, and
  each test writes it to the disk before ffmpeg can read it.

## What converges

- Committed files: in all but 3. The decisive one is Tim's rule.
- One home that every package's tests share: 2 and 5. 1 copies the test
  picture or reaches across packages by path.
- A plain file on the disk, found by its path: 1, 2 and 4. ffmpeg, the
  decoder and the walks' browser read files.
- The way each was made, kept beside it: 1, 2 and 3. A fixture made
  again on purpose comes out the same, and a reader sees what it is.

Dropped: LFS, which only one needs and which costs every machine a tool,
and embedding, which only one needs and makes every test write the file
again.

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
- Each is made by its recipe in `internal/fixtures/fixtures_test.go`, and
  `go test ./internal/fixtures -update` makes them all again. Without the
  flag, that test checks the folder holds the recipes and nothing else.
- As few as the tests allow: a new test reads one that is there, unless
  what it tests is a property none of them has. Then it adds one, as
  small and as short as the test allows.
- `engine/testdata/open-gop.mp4` and `cmd/framefairy-app/testdata/gnome.webm`
  move in as their packages' tests move.
- The move is made over several pull requests, in the order of #670's
  Progress list. Until it is done, what is left is under Not to copy in
  ARCHITECTURE.md.

## What follows

- Costs: the repository grows by the fixtures, about 4 MB for the
  engine's shared test video, and each change to one is a new copy in
  Git's history. A fixture is made small before it is committed.
- Buys: no ffmpeg run to make an input, so no input made on CI's
  graphics chip, and the time it took. The tests read the same bytes on
  every machine, so a failure is about the code, not about the ffmpeg
  that made its input.
- Wrong if: the repository grows past what a clone takes in a few
  seconds, or a fixture has to change with nearly every pull request.
  Then the fixtures that change are made by the tests again, and the
  rest stay.
