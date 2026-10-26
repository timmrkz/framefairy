package workflows

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every workflow reads as YAML. Most of them run only on main, at night or
// by hand, so a broken one would otherwise be seen only after the merge.
//
// On the real disk: what is read is the repository's own workflows.
func TestEveryWorkflowReads(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("found no workflow in .github/workflows")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Errorf("%s does not read as YAML: %v", filepath.Base(f), err)
			continue
		}
		if doc["jobs"] == nil {
			t.Errorf("%s has no jobs", filepath.Base(f))
		}
	}
}
