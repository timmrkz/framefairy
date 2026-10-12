// Package offdisk holds the check that keeps the tests off the disk. A
// test of our own code works on files in memory, an afero.MemMapFs, and
// one that goes to the real disk says why in a comment that starts with
// "On the real disk", see CLAUDE.md and engine/files.go. The check is a
// test, so it runs wherever the tests run, in make test and in CI.
package offdisk
