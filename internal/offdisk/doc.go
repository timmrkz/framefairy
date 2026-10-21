// Package offdisk holds the check that keeps the tests off the disk. A
// test of our own code works on files in memory, an afero.MemMapFs, and
// one that goes to the real disk says why in a comment that starts with
// "On the real disk", see CLAUDE.md and engine/files.go. A second check
// over the same files holds every program a test writes and runs to
// standin.Write, see internal/standin. The checks are tests, so they run
// wherever the tests run, in make test and in CI.
package offdisk
