// Package dispenser is the half of the licence system that is always on
// and cannot make a key. It holds a pool of keys signed in advance, hands
// the next one to each sale, and keeps a record of everything it did. Every
// rule about sales lives in its Engine. What it needs from outside, the
// database, the shop and the mail, is behind small interfaces, so the
// engine is tested in memory with a fixed clock. See docs/LICENCE.md.
package dispenser
