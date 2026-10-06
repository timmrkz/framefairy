//go:build production

package engine

// shipped says this is the app as it is built to ship, made with the
// production tag, see the Makefile. The tests and the command line are
// built without it.
const shipped = true
