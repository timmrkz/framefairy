# Robustness

The engine, the app's Go side and the interface hand work to each other all
the time. This is where the handoffs that can go wrong are written down, with
what was done about each. The rule: **a slower or poorer moment is fine, a
frozen, broken or stuck app is not.** The batches are the robustness track in
[GUI-PLAN.md](GUI-PLAN.md).

Every fix here starts as a test that makes the failure happen, run under
`go test -race` and from several goroutines at once, and fails on the code
before the fix.

## The audit

Read from the code, most severe first. Each line says where, what happens and
what became of it.

1. **Removing an episode during its first search.** The search paused the
   episode's transcription and carries it on when it is stopped, so removing
   the episode started a new transcription of it. The removal waited for that
   in vain, refused to delete anything, and a transcription ran on for an
   episode that was gone. The open workspace could ask for work in the same
   gap. Fixed: an episode being removed is closed in the queue, nothing new is
   queued for it until it is added again, and the ask comes back as a job that
   failed with the reason. Removing it and keeping its work stops its jobs
   too. `cmd/framefairy-app/jobs.go`, tests in `remove_test.go`.
2. **Two language models loaded at once.** When the model in memory is held
   and an ask needs another model or more room, a second server is started
   beside it, about twice the memory. It happens when a warm-up for one
   episode meets a search of another, or when a search asks for more room
   while a warm-up is still loading. Fixed: one model at a time, never two.
   An ask that needs another model or more room waits for the one in memory
   to be let go of and then takes its place, and a warm-up, which is only a
   head start, gives way to a model in use. The test that holds models from
   24 goroutines at once had 6 in memory together before. `engine/modelhost.go`.
3. **Servers left running after quitting.** A model still loading when the
   app quits, a second server from the point above, and a render's ffmpeg are
   not stopped with the app. llama-server holds its memory until it is killed.
   Fixed: quitting stops every job, and with it any ffmpeg, then stops the
   model whether it is loaded or still loading, and waits for both, at most
   15 s. `engine/modelhost.go`, `cmd/framefairy-app/jobs.go`,
   `cmd/framefairy-app/main.go`. Still open: an app that is killed rather
   than quit, or that crashes, leaves llama-server running, because nothing
   is left to stop it.
4. **A panic outside a job's own goroutine ends the app.** Framing clips and
   the search clock run on goroutines of their own with no recover, so a
   panic in them is not caught by the job. The lanes of the queue die of a
   panic in what receives their news. Fixed: a clip that panics while it is
   framed is left out with a warning and the search goes on, a clock that
   panics stops reporting, and a lane survives whatever happens around a
   job, its news included. `engine/planbuild.go`, `engine/searchclock.go`,
   `cmd/framefairy-app/jobs.go`, each with a test that crashed the test
   program before.
5. **A job event lost or out of order leaves the interface stuck.** The
   queued event can arrive after the running one, events sent before the
   interface starts listening are lost, and nothing reads the job list again.
   A job then looks as if it runs for ever and blocks the first search.
   Fixed: every snapshot of a job carries a number that grows with every
   change and is set under the queue's lock, and the interface keeps the
   snapshot with the larger number, whatever order they arrive in. It
   listens before it reads the list, reads the list again every 5 s while
   anything runs, and a cancel is always answered, also for a job that had
   already ended. The reordering is read from the code: 25 changes from 8
   goroutines at once did not make it happen in five runs.
   `cmd/framefairy-app/jobs.go`, `frontend/src/lib/state.svelte.ts`,
   `frontend/src/lib/flow.ts`.
6. **Work waiting behind work.** A search waiting for a transcript holds the
   work lane, and every render of every episode waits behind it. Slower,
   not stuck. Carrying on paused transcriptions held it up to 15 s more,
   fixed with the next point. The rest is open, R.2.
7. **A transcription paused for good.** If a paused transcription takes more
   than 15 s to stop, it is not carried on, and the next search fails with a
   pause nobody made. Fixed: a transcription that has not stopped within a
   second is waited for in the background, for as long as it takes, and
   carried on then. The search's way out no longer waits for it.
   `cmd/framefairy-app/main.go`, test in `pause_test.go`.
8. **Clip lists landing out of order in the interface.** An older list that
   arrives after an edit writes over it, so the edit looks undone until the
   next refresh. `frontend/src/screens/Episode.svelte`. Open, R.6.
9. **Undo taking a clip a search found.** A clip that lands while an edit is
   saved becomes part of that edit, and undoing it removes the clip.
   Fixed: an edit changes clips and never makes them, so a clip or a plan
   that appears between the two pictures of an edit is left out of it.
   `engine/undo.go` `LeaveOutNewClips`, used by the history in
   `cmd/framefairy-app/history.go`.
10. **Two settings changes at once lose one.** Settings are read, changed and
    written back outside the store's lock. Fixed: every change is one step
    under the lock, `UpdateSettings`. The test lost the number of clips in
    two runs out of three on the old code. `cmd/framefairy-app/settings.go`.
11. **Smaller ones.** Stopping llama-server read its state while another
    goroutine wrote it, fixed in `engine/local.go`. A server that crashes after
    loading costs one search.
    A warm-up for a search that failed still loads the model. The job list,
    the probe cache and the plan locks grow while the app is open. Open.
