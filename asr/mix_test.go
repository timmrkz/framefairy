package asr

import "testing"

// The pool is sized by the machine underneath, the way it was measured.
func TestMixFollowsTheMachine(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		name            string
		m               Machine
		copies, threads int
	}{
		// Measured: four copies of 2 on the 8 performance cores.
		{"M2 Max, 32 GB", Machine{Cores: 12, Fast: 8, Memory: 32 * gb}, 4, 2},
		// Measured: a copy a core with one thread each.
		{"M1 runner, 3 cores", Machine{Cores: 3, Memory: 7 * gb}, 1, 3},
		{"cloud, 4 cores, 16 GB", Machine{Cores: 4, Memory: 16 * gb}, 2, 2},
		{"cloud, 4 cores, 16 GB as Linux says it", Machine{Cores: 4, Memory: 15_600_000_000}, 2, 2},
		{"cloud, 4 cores, memory unknown", Machine{Cores: 4}, 4, 1},
		// Memory holds the copies back, and the threads go to the rest.
		{"M1, 8 GB", Machine{Cores: 8, Fast: 4, Memory: 8 * gb}, 1, 4},
		{"M2 Pro, 16 GB", Machine{Cores: 12, Fast: 8, Memory: 16 * gb}, 2, 4},
		// Never more copies than measured, never a machine with no thread.
		{"a big one", Machine{Cores: 32, Fast: 24, Memory: 192 * gb}, 4, 6},
		{"nothing known", Machine{}, 1, 1},
	} {
		copies, threads := Mix(c.m)
		if copies != c.copies || threads != c.threads {
			t.Errorf("%s: %d copies of %d threads, want %d of %d",
				c.name, copies, threads, c.copies, c.threads)
		}
	}
}
