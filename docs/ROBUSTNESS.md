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
   while a warm-up is still loading. `engine/modelhost.go`. Open, R.4.
3. **Servers left running after quitting.** A model still loading when the
   app quits, a second server from the point above, and a render's ffmpeg are
   not stopped with the app. llama-server holds its memory until it is killed.
   `engine/local.go`, `engine/modelhost.go`, `cmd/framefairy-app/main.go`.
   Open, R.4.
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
   `cmd/framefairy-app/jobs.go`, `frontend/src/lib/state.svelte.ts`. Open, R.2
   and R.6.
6. **Work waiting behind work.** A search waiting for a transcript holds the
   work lane, and every render of every episode waits behind it. Carrying on
   paused transcriptions holds it up to 15 s more. Slower, not stuck. Open,
   R.2.
7. **A transcription paused for good.** If a paused transcription takes more
   than 15 s to stop, it is not carried on, and the next search fails with a
   pause nobody made. Open, R.3.
8. **Clip lists landing out of order in the interface.** An older list that
   arrives after an edit writes over it, so the edit looks undone until the
   next refresh. `frontend/src/screens/Episode.svelte`. Open, R.6.
9. **Undo taking a clip a search found.** A clip that lands while an edit is
   saved becomes part of that edit, and undoing it removes the clip.
   `cmd/framefairy-app/history.go`. Open, R.5.
10. **Two settings changes at once lose one.** Settings are read, changed and
    written back outside the store's lock. Open, R.2.
11. **Smaller ones.** Stopping llama-server reads its state while another
    goroutine writes it. A server that crashes after loading costs one search.
    A warm-up for a search that failed still loads the model. The job list,
    the probe cache and the plan locks grow while the app is open. Open.
