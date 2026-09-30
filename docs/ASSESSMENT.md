# Assessment

A review of the whole repository for design and engineering practice, made
on 29 September 2026 against main at `897f1ea`. It covers the engine, the
app's Go side, the interface, the build, CI and the tests. Each item says
where, what happens, what to do and where it stands, most severe first
within each group. Line numbers are as found and drift as the code moves.

The work is done in batches, one pull request each, and the status here is
updated in the same pull request as the fix. [ROBUSTNESS.md](ROBUSTNESS.md)
is the older audit of the handoffs between the parts, and nothing here
repeats it.

**Status:** Open, In progress (with its pull request), Fixed, Decided (a
choice made, with what was chosen), or For Tim (waits on a decision).

## Overall

The code is in good shape. `go vet` and `gofmt` are clean, the race tests of
the job queue pass, coverage is 85% in the engine, 90% in `train` and 87% in
`updates`. Model answers, plan files and paths go through the checks
`CLAUDE.md` asks for. Every child process is started with a context and
waited on. API keys live in the macOS Keychain and are never logged. The
updater checks an Ed25519 signature with a key built into the app. The
ffmpeg build is kept LGPL and that is checked on the binary itself.

What is wrong is a handful of real bugs, one hole in how builds are signed,
and design debt that has grown with the code: errors carried as log text, a
facade the app goes around, and three files too big to hold in one's head.

## Glaringly wrong

1. **Every branch build can read the update signing key.** `builds.yml`
   builds every pushed branch that has a pull request. It put
   `FRAMEFAIRY_UPDATE_KEY` in the environment and ran
   `go run ./cmd/framefairy-release sign` from the branch's own code. Every
   development build trusts that key (`cmd/framefairy-app/updates.go:32`).
   Customers get a release key of their own, as
   [UPDATES.md](UPDATES.md) plans, so this reaches development builds
   only, which is less than the first draft of this item said. Still, any
   branch, one pushed by a Claude session gone wrong included, could sign
   anything or send the key elsewhere. `publish` uploaded `dist/*` with
   `--clobber`, so a branch build could also replace main's
   `channel-main.json`, and the `list` and `announce` jobs ran
   `scripts/channel-list.sh` from the branch with a token that can write.
   A change to the YAML alone cannot close this, because a push runs the
   workflow file of the branch pushed, and a branch can rewrite it. Fixed
   in #43: `builds.yml` gets no secret and no token that can write, and
   leaves only the zip. `publish.yml`, which GitHub always runs from main,
   works out channel, commit and version from GitHub's record of the run,
   signs, and uploads the channel's own two files by name. The key lives
   in the environment `updates`, limited to main, and the repository
   secret is gone. **Fixed** in #43 and #46.
2. **A short counts as rendered while it is written, and after it is cut
   off.** Render writes straight to `out/<name>.mp4`
   (`engine/render.go:164`), and a clip counts as rendered as soon as that
   file exists (`engine/episode.go:447`). While it renders, the clip already
   reads as rendered. After a crash mid-render, or a length check that fails
   (`render.go:204`), a broken file stays and counts as done. A failed
   re-render deletes the good short from before. Fix: write to
   `<name>.part.mp4`, check it, then rename it into place, the way
   thumbnails already do (`render.go:251-268`). **Fixed** in #44:
   `<name>.part.mp4` is checked and then renamed into place, and nothing
   that counts shorts takes a `.part` file for one.

## Bugs

3. **A panic while hearing holds the hearing lane until the app restarts.**
   `hear` in `engine/jobs.go:479` calls `release()` after `p.Hear` returns,
   not in a `defer`. A panic fails the job, but the lane stays taken, and
   every later search and clip made by hand waits for good. That breaks the
   rule that a job that goes wrong fails itself, not the app. Fix: release
   in a `defer`, as the other steps do. **Fixed** in #44.
4. **Edits do not check that a plan belongs to its episode.** `SetWord`,
   `SetCrop`, `ResetCrop`, `SetThumbnail`, `SetCaptionTime`, `RemoveClip`,
   `Shape`, `Reshape`, `Captions` and the caption setters
   (`cmd/framefairy-app/main.go:1027` and on) only check that each file is
   somewhere in the library. A plan of episode B with the path of episode A
   is accepted: the undo history snapshots the wrong episode, the crop is
   clamped to the wrong video and the words come from the wrong transcript.
   `RemoveSearch` already checks it properly. Fix: one guard used by every
   edit. **Fixed** in #44: `PlanOf` checks the plan is in the episode's
   own logs folder, and every edit, the captions and the render go
   through it.
5. **CI skips the Go tests for a change to the interface alone.**
   `scripts/ci-needs.sh:86` and `scripts/changed.sh:104` see no Go change,
   but `cmd/framefairy-app/bindings_test.go:115` reads
   `frontend/src/lib/api.ts` and `engine/suggest_test.go:36` reads
   `frontend/src/lib/suggest.cases.json`. A break between the two sides
   passes the pull request and fails on main. Fix: count the files the Go
   tests read as Go changes, in both scripts and in `ci-needs-test.sh`.
   **Fixed** in #44: such a file is found by the path a test reads it by,
   so a new one needs no new rule.
6. **Starting the app can kill a llama-server the command line is using.**
   The CLI and the app share one note, `~/.framefairy/llama-server.json`, and
   `StopLeftoverServer` (`engine/leftover.go:88`) stops whatever matches its
   port and model, live or not. Either can also overwrite the other's note,
   so a real leftover is then never found. Fix: keep the owner's process id
   in the note and stop the server only when that owner is gone. **Fixed**
   in #48: one note a server, with the process that started it, and a
   model path with a space in it is found too.
7. **The model's health check cannot be cancelled.** `local.go:326` asks
   `/health` with a client that has no timeout and no context. A server that
   accepts and never answers holds the load, and at quit it is left running
   until the next start. Fix: a request with the context and about 2 s per
   try. **Fixed** in #48.
8. **The retry for a model that spent its budget thinking depends on the
   wording of an error.** `engine/api.go:705` looks for
   `"blocks: ['thinking']"` in `err.Error()`. Changed wording, or a reply
   with another block in the list, turns the retry off without a sound. Fix:
   a sentinel error checked with `errors.Is`. **Fixed**
   in #48: a reply of thinking blocks alone, shown or hidden, carries
   `errSpentThinking`.
9. **A window's start goes from number to text and back.** `Project.Plan`
   names the plan with `int(window.Start)` (`engine/project.go:139`), then
   hands `Run` the start as a string with three decimals, which `Run` parses
   and names the file from again (`run.go:218, 266`). A start of 29.9996
   gives the caller `clips-29-…` while `Run` writes `clips-30-…`. The
   interface usually snaps windows, so it is latent. Fix: pass the window as
   a value. **Fixed**
   in #48, differently: the window is rounded to the millisecond before
   anything names it, in one function, `searchWindow`, which the search,
   its record and Run's input now share. A job record also named the
   whole episode asked for by its end `clips-0-<end>.json` where the
   search wrote `clips.json`.
10. **Engine logic copied into the app has drifted.** Finding ffprobe beside
    a chosen ffmpeg is done twice in the app (`main.go:391, 647`), without
    the `ffprobe.exe` case the engine has (`engine/run.go:145-155`), so on
    Windows the app would not find it. `main.go:436` repeats `tokens.txt`
    from `engine/speech.go:98`. Fix: one engine helper used by both. **Fixed** in #48: `Engine.UseTools` and
    `SpeechModelReady`, used by the command line and the app alike. The
    app's own check had also taken an empty `tokens.txt` for a model.

## Risks

11. **The sources of the programs we ship are not checked.**
    `scripts/build-ffmpeg.sh:108-133` downloads five tarballs with no
    checksum, and it and `build-llama.sh:78` clone by tag, which can be
    moved. `pack-source.sh` fetches the "corresponding source" separately,
    also unchecked, so nothing proves it is what was built, which LGPL asks
    for. `speech-libs.sh` already does it right. Fix: a sha256 per tarball
    and a commit per tag, checked in both scripts. **Fixed** in #51: the four archives are
    checked against a sha256, ffmpeg and llama.cpp against the commit of
    their tag, in the build and in `pack-source.sh` alike. ffmpeg is now
    always taken from git, since ffmpeg.org publishes no checksum to hold
    its archive to.
12. **ffmpeg 7.1.1 is four point releases behind.** 7.1.5 is out on the same
    branch, with security fixes in code that reads the user's video.
    harfbuzz 10.1.0 and libass 0.17.3 want a look too. **Fixed** in #54: ffmpeg 7.1.5.
    The hang did not come back: on a macOS runner, 80 runs of the command
    and three rounds of the engine tests under the race detector, with
    7.1.1 and with 7.1.5 alike, never hung, and the macOS CI passed with
    7.1.5. So it was rare rather than new. What made it matter was ours:
    nothing ended an ffmpeg that had stopped. Looking for camera switches
    now has a time limit, falls back to the processor when the system's
    decoder runs out of it, and frames a span without switches, with a
    warning, when both do. harfbuzz 10.1.0 and libass 0.17.3 stay.
13. **Workflows are not hardened.** `ci.yml` and `speechbench.yml` have no
    `permissions:` block, every action is pinned by tag rather than by
    commit, and `tools.yml` puts `inputs.tag` straight into a shell line.
    Fix: `contents: read` by default, pins by commit or Dependabot for
    actions, inputs through `env:`. **Fixed** in #51: every
    workflow has a `permissions:` block, every action is pinned to the
    commit its tag named on 29 September 2026, with the tag beside it,
    and the release tag reaches the shell through `env:`. Pinned actions
    do not update themselves. Dependabot could propose updates as pull
    requests, which is left for Tim to decide.
14. **Writes that replace a file are not flushed.** `writeAtomic`
    (`engine/transcript.go:140`) and `replacePlan` (`engine/edit.go:236`)
    write a temporary file and rename it, but never sync it. After a power
    loss a plan or transcript can come back empty. `transcript.go:134-137`
    also writes the frames file and the JSON as two steps. Fix: sync before
    the rename, and make the two one function. **Fixed** in #51: one
    `replaceFile`, which flushes the file before the move and the folder
    after, and which `writeAtomic` and `replacePlan` both go through. The
    frames and the transcript stay two files written one after the other.
15. **Plan locks hold only inside one process.** `lockFile`
    (`engine/edit.go:151`) is a map of mutexes, so the CLI and the app
    editing one plan at once can lose an edit. Unlikely, but it should be
    written down or closed with a file lock. **Fixed** in
    #51 on macOS and Linux: `flock` on a hidden file beside the plan or
    transcript, taken after the lock inside the program. Windows keeps
    only the lock inside the program until it is shipped.
16. **Some reads and downloads have no bound.** The speech model download
    uses `http.DefaultClient` (`engine/speech.go:223`), so a stalled
    connection waits for Cancel. `api.go:297` reads a reply with
    `io.ReadAll` and no limit. Fix: a timeout for stalls, and
    `io.LimitReader` of a few MB. **Fixed** in #51: a download that gets no
    byte for a minute ends, with what came kept to carry on from, and an
    answer read whole is read to 16 MB.
17. **No fuzz target for the model's streamed answers.** `readClaudeStream`
    (`engine/stream.go:135`) and `readLocalStream` (`:417`) read what a model
    sends, on the default paths, and have unit tests only. `CLAUDE.md` asks
    for fuzz targets on everything that reads a model answer. **Fixed** in #51:
    `FuzzReadClaudeStream` and `FuzzReadLocalStream`.

## Design debt

18. **Errors travel as log lines.** `Engine.Run` is 518 lines long
    (`engine/run.go:136-654`) and returns an exit code. `Project` gets the
    reason back by hooking the last error line (`project.go:57-76`) and the
    job records it (`jobs.go:413`). So the kind of error is lost, only one
    `fmt.Errorf` in the engine wraps with `%w`, and the app shows messages
    written for the command line, like "--planner must be local or api".
    `planFailed` has two identical branches (`run.go:767-773`). Fix: `Run`
    returns an error and the command line turns it into an exit code. Split
    `Run` into steps `Project` calls directly instead of filling `Options`
    with strings. **Open.**
19. **`Project` covers part of what the app needs.** The app calls 92 engine
    functions directly, and builds `engine.NewProject(nil, …)` seven times
    only to get paths. Such a project panics if asked to run anything. Fix:
    an episode type for paths, reads and edits, apart from the part that
    runs work and needs an engine. **Open.**
20. **A clip in a plan has five shapes.** `Clip` (`clips.go:30`), `PlanClip`
    (`plan.go:122`), the ordered object of `edit.go` and `shape.go`,
    `Plan.Raw` and `PlanEntry` (`select.go:247`), with key names written out
    as strings in nine files. The ordered object is needed to keep unknown
    fields, the rest goes against "one primitive per thing". Fix: the key
    names as constants and one way to read and write a clip. **Open.**
21. **The engine is one package of 57 files and 21,000 lines**, with 248
    exported names, 53 of the functions used nowhere outside it. Core types
    sit in unrelated files (`Engine` in `ffmpeg.go`, `Transcript` in
    `audio.go`, `Window` in `plan.go`), and `asr` imports `engine` for
    `Token` and `Recognizer`, the wrong way round. Splitting it now would
    make import cycles. Fix, in order: unexport what nobody outside uses,
    move core types to files of their own, then a small shared package, then
    the typesetting and the model providers. **Open.**
22. **Global state keeps the engine tests serial.** `trainingDir` is set from
    inside `Run` (`run.go:140`), there is a package-level model host and
    several test hooks, and `DefaultStyle` is an exported map anyone can
    change. No engine test runs in parallel, and the engine tests take four
    minutes. **Open.**
23. **`Episode.svelte` is 3,100 lines.** Its script has 45 `$state`, 86
    `$derived` and 17 `$effect`. Most effects watch for a job to end by
    keeping the value before in a plain variable, which makes the job's
    life a state machine spread over effects, untested. Fix: the episode
    session as a runes class in `lib/`, with tests, and job endings reported
    by the job store. `ClipTimeline` gets about 30 props and `Player` about
    22, and `showChosen` reaches into ClipList's markup by class name. **Open.**
24. **`cmd/framefairy-app/main.go` is 1,600 lines with 70 bindings.** The
    tests are already split by the right seams, the code is not. Fix: media,
    views, words, edits, chosen and training in files of their own,
    `CheckSetup` into `setup.go`. **Open.**
25. **The TypeScript types are written by hand.** The binding test checks
    names and argument counts, not fields. Constants copied from the engine,
    the caption heights and `snapCaptionY`, the caption defaults, `Reach`
    and `fitWindow`, have no shared test. `suggest.cases.json` shows the way:
    one file of cases both sides read. **Open.**
26. **Two ways of saving settings.** `SaveSettings` replaces everything and
    has to carve out `Chosen` by hand (`main.go:367-373`), while other calls
    change one field. Each new field the Go side owns needs another carve
    out or is lost to the last save. Fix: saves that change only what
    changed. **Open.**
27. **One name, two things.** `engine.Compare` compares two undo snapshots,
    `Engine.Compare` compares recipes. **Open.**

## Against the project's own rules

28. **The error line over the workspace is a banner.** `problem` in
    `Episode.svelte` is set in about 40 places and shown above the
    workspace (2084-2092), render failures included, which "no toasts and
    no banners" rules out. It is measured and written back as `--above` on
    every resize, since it wraps. Fix: each failure in the row of the work
    that failed. **Open.**
29. **Settings has a Training data section** (`Settings.svelte:812-834`,
    bindings at `main.go:1571-1609`), while `CLAUDE.md` says no training
    screens in the app and everything about the records in
    `framefairy-train`. **Decided:** it stays for now, as not important
    yet. It comes up again with 5.7 in GUI-PLAN.md, whether customer
    builds record training data at all.
30. **Sizes worked out in JavaScript.** `RangeWindow.svelte:114` measures its
    width and places everything in pixels, to land on whole pixels.
    `Player.svelte:702` does the same for the caption scale. CSS `round()`
    and container units would do both in the stylesheet. **Open.**

## Small things

31. Dead code: `wrapText` (`ass.go:295`), `captionLayout`
    (`highlight.go:66`), `storedForm` (`transcript.go:358`, with a comment
    that says it matters), `medianIndex` (`util.go:304`), the `ReadPlan`
    binding and `api.readPlan`, and the `tslib` dev dependency. **Fixed** in #58.
32. Errors worded like Python reach the user: `'start'` (`clips.go:311`) and
    "could not convert string to float" (`clips.go:161`). **Fixed** in #58.
33. Failures in the app come back as `os.ErrNotExist`, "file does not
    exist", where `notInLibrary` says what happened. **Fixed** in #58.
34. `make help` prints lines 1 to 39 of the Makefile (line 117) and the
    header runs to 45, so it stops mid-sentence. **Fixed** in #58.
35. Docs that drifted: the fuzz target count and the `TIDY=0` step in
    BUILD.md, the header of `build-ffmpeg.sh` that says make does not run
    it, the two CI jobs in WORKFLOW.md where there are six, and 18 files
    missing from the code map in ENGINE.md. **Fixed** in #58.
36. The ffmpeg and llama.cpp licence files never reach the bundle:
    `tools-beside` copies only the binaries, so the loop in
    `bundle-macos.sh:61` finds nothing. **Fixed** in #58.
37. Tests that wait on time: `remove_test.go:169, 190, 236` sleep a fixed
    300 ms for a 200 ms job. Poll with a deadline instead. **Fixed** in #58.
38. When our ffmpeg build fails, `tools.sh:104` unlinks the user's Homebrew
    ffmpeg and installs the tap's, a change to the machine nobody would
    expect. **Fixed** in #58.
39. No tests in `cmd/framefairy`, `cmd/framefairy-train` or
    `cmd/framefairy-release`, which signs updates. No check that `go.mod` is
    tidy in CI, since make tidies it quietly. **Fixed** in #58.
40. `Reveal` (`main.go:1547`) and the other `.Start()` calls never `Wait`,
    so each leaves a zombie until the app quits. **Fixed** in #58.
41. Colours and heights outside the tokens: `rgba(224,96,90,.14)` in
    `ClipList.svelte:344` is `--err` written out, `--wave` is defined in
    `ClipTimeline.svelte` with two different values, the backdrop
    `rgba(0,0,0,.55)` is repeated in three components, and the card height
    `56px` four times in `ClipList.svelte`. **Open.**
42. Doc comments in the wrong place: `ResetCrop`'s above `SetCaptionStyle`
    (`main.go:1064`), an orphan above `Notice` in `api.ts:10`. **Fixed** in #58.
43. `make changed` runs no Go test for a change to a build script.
    `notices_test.go` checks the versions in `notices.json` against the pins
    in `build-ffmpeg.sh` and `build-llama.sh`, but it names them without
    their folder, so the rule that finds a Go test by the path it reads a
    file by never finds it. Raising ffmpeg in #54 passed `make changed` and
    failed the notices test in CI. **Fixed** in #58.
