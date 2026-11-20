// Package maccode holds the check that keeps the code that behaves
// differently on a Mac in files named for it, *_darwin.go and
// *_darwin_test.go. The compiler builds such a file only for macOS, and a
// pull request runs the Go tests on a Mac only when it changes one, see
// the job macos in .github/workflows/ci.yml. So the check is what makes
// that filter exact: a file that builds on a Mac and not on Linux is named
// for the Mac, and no other file asks runtime.GOOS a question a Mac and
// Linux answer differently. The check is a test, so it runs wherever the
// tests run, in make test and in CI. See the decision record
// docs/decisions/2026-11-20-a-mac-only-for-mac-code.md.
package maccode
