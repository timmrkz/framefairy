package main

import (
	"io"
	"slices"
	"testing"
)

// toolRelease is the release release-tool as a test needs it.
type toolRelease struct {
	files []asset
	data  map[string][]byte
}

func (r *toolRelease) assets() ([]asset, error) { return slices.Clone(r.files), nil }

func (r *toolRelease) upload(name string, data []byte) error {
	r.files = slices.DeleteFunc(r.files, func(a asset) bool { return a.name == name })
	r.files = append(r.files, asset{name: name})
	r.data[name] = data
	return nil
}

func (r *toolRelease) remove(a asset) error {
	r.files = slices.DeleteFunc(r.files, func(b asset) bool { return b.name == a.name })
	delete(r.data, a.name)
	return nil
}

// The builds kept in the release replace the ones there, and take the
// builds named by a key from before away, so the release holds the two
// files the jobs download, one for Linux and one for a Mac.
func TestKeepingABuildLeavesItAlone(t *testing.T) {
	t.Parallel()
	r := &toolRelease{
		files: []asset{{name: toolName}, {name: "framefairy-release-0123456789abcdef"}},
		data:  map[string][]byte{toolName: []byte("older"), "framefairy-release-0123456789abcdef": []byte("keyed")},
	}
	if err := keepTool(r, map[string][]byte{toolName: []byte("newer"), toolMac: []byte("for a Mac")}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(r.files) != 2 || string(r.data[toolName]) != "newer" || string(r.data[toolMac]) != "for a Mac" {
		t.Errorf("the release holds %v, want only the two new builds", r.files)
	}
}
