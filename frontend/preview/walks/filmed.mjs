// The walks and sequences that record a film of the screen in their
// trace, each with the issue it fails now and then in. Every walk records
// a trace, with every call of the walk, what the page logged and threw
// and every request to the Go side. The film costs more, every walks job
// in CI a fifth longer when every walk had it, so a walk has it only when
// its trace showed that what the screen looked like is still missing.
//
// A walk is named here when its trace was not enough, and the pull
// request that fixes it takes its line out again. A sequence is named by
// its name, a walk by its file, for every seed, or by its file and seed
// as it prints them, for that one. sequences.mjs --names fails on a name
// here that is no walk or sequence, so a line cannot outlive what it
// names. TRACE=film films every walk for one run, TRACE=none records no
// trace at all. See Finding why a walk failed in docs/TESTING.md.
export const filmed = {};
