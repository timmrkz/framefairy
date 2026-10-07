// Package outside checks on a Mac what starts outside the app: a link in
// a mail that opens it, closed or already open. Its tests only build with
// the tag outside, and only make outside runs them, in CI, against a
// bundle built with the probe. See From outside the app in
// docs/TESTING.md.
package outside
