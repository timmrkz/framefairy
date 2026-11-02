package fixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// update makes the fixtures that are missing or whose recipe changed,
// with go test ./internal/fixtures -update, the way Go's golden files are
// made again. Without it the test only checks that the files are the
// recipes. A file whose recipe did not change is left as it is, so Git
// keeps no second copy of it made by another ffmpeg.
var update = flag.Bool("update", false, "make the fixtures that are missing or whose recipe changed")

// sums is the file in testdata that says which recipe each fixture was
// made by, one line each, the name and the sum of its recipe.
const sums = "recipes.sum"

// recipes are the arguments ffmpeg makes each fixture with, the output
// path last. mpeg4 and aac are in every ffmpeg, the one we ship included,
// which has no libx264.
var recipes = map[string][]string{
	// The test picture with its counter and a tone, at the three lengths
	// the engine's tests of a search need: a video short enough to be its
	// own clip, one a window covers, and one longer than a window.
	"episode-15s.mp4": episode("15"),
	"episode-40s.mp4": episode("40"),
	"episode-70s.mp4": episode("70"),
}

func episode(seconds string) []string {
	return []string{
		"-f", "lavfi", "-i", "testsrc=s=320x180:r=25:d=" + seconds,
		"-f", "lavfi", "-i", "sine=f=220:d=" + seconds,
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac",
	}
}

// recipeSums is the line of recipes.sum each recipe should have.
func recipeSums() string {
	var lines []string
	for name, args := range recipes {
		sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
		lines = append(lines, name+" "+hex.EncodeToString(sum[:8]))
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

// madeBy reads recipes.sum: the sum of the recipe each fixture was made
// by.
//
// On the real disk: recipes.sum is committed beside the fixtures.
func madeBy(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(Dir(), sums))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	out := map[string]string{}
	for line := range strings.Lines(string(body)) {
		if name, sum, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			out[name] = sum
		}
	}
	return out
}

// On the real disk: the fixtures are files in testdata, and with -update
// ffmpeg writes them there.
func TestEveryFixtureIsMadeByItsRecipe(t *testing.T) {
	want := map[string]string{}
	for line := range strings.Lines(recipeSums()) {
		name, sum, _ := strings.Cut(strings.TrimSpace(line), " ")
		want[name] = sum
	}
	if *update {
		had := madeBy(t)
		for name, args := range recipes {
			path := filepath.Join(Dir(), name)
			if _, err := os.Stat(path); err == nil && had[name] == want[name] {
				continue
			}
			made := append(append([]string{"-loglevel", "error", "-y"}, args...), path)
			if out, err := exec.Command("ffmpeg", made...).CombinedOutput(); err != nil {
				t.Fatalf("making %s: %s %s", name, err, out)
			}
			t.Logf("made %s", name)
		}
		if err := os.WriteFile(filepath.Join(Dir(), sums), []byte(recipeSums()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		t.Fatal(err)
	}
	var there, wanted []string
	for _, e := range entries {
		if e.Name() != sums {
			there = append(there, e.Name())
		}
	}
	for name := range recipes {
		wanted = append(wanted, name)
	}
	slices.Sort(there)
	slices.Sort(wanted)
	if !slices.Equal(there, wanted) {
		t.Errorf("testdata holds %v, the recipes make %v. go test ./internal/fixtures -update makes them again", there, wanted)
	}
	had := madeBy(t)
	for _, name := range wanted {
		if had[name] != want[name] {
			t.Errorf("%s was not made by its recipe as it is now. go test ./internal/fixtures -update makes it again", name)
		}
	}
	if len(had) != len(want) {
		t.Errorf("%s names %d fixtures, the recipes make %d", sums, len(had), len(want))
	}
}
