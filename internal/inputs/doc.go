// Package inputs holds the early warning that the app does not start to
// watch input it was not given, issue #369: our Go code never registers a
// global shortcut, which hears keys pressed in other apps, and the
// interface never opens the microphone, the camera or the screen itself
// or reads the clipboard by itself. The wall that holds for a build is
// the check before it is signed, see cmd/framefairy-release/inputs.go.
// This one is a test, so it runs in the pull request that would start it.
package inputs
