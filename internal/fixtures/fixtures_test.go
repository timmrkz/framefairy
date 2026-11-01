package fixtures

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// update makes every fixture again from its recipe, with
// go test ./internal/fixtures -update, the way Go's golden files are
// made again. Without it the test only checks that the files are the
// recipes.
var update = flag.Bool("update", false, "make every fixture again from its recipe")

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

// On the real disk: the fixtures are files in testdata, and with -update
// ffmpeg writes them there.
func TestEveryFixtureIsMadeByItsRecipe(t *testing.T) {
	if *update {
		for name, args := range recipes {
			made := append(append([]string{"-loglevel", "error", "-y"}, args...), filepath.Join(Dir(), name))
			if out, err := exec.Command("ffmpeg", made...).CombinedOutput(); err != nil {
				t.Fatalf("making %s: %s %s", name, err, out)
			}
		}
	}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		t.Fatal(err)
	}
	var there []string
	for _, e := range entries {
		there = append(there, e.Name())
	}
	var wanted []string
	for name := range recipes {
		wanted = append(wanted, name)
	}
	slices.Sort(there)
	slices.Sort(wanted)
	if !slices.Equal(there, wanted) {
		t.Errorf("testdata holds %v, the recipes make %v. go test ./internal/fixtures -update makes them again", there, wanted)
	}
}
