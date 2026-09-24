package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovingAModelLeavesNothingOfIt(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lm := LanguageModel{Name: "a.gguf"}
	sm := SpeechModel{Name: "speech"}
	write("a.gguf")
	write("a.gguf.part")
	write("b.gguf")
	write("speech/tokens.txt")
	write("speech.part")
	write("speech.unpacking/x")
	if err := RemoveLanguageModel(lm, dir); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSpeechModel(sm, dir); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 1 || left[0].Name() != "b.gguf" {
		t.Errorf("left behind: %v", left)
	}
	// Removing what is not there is fine.
	if err := RemoveLanguageModel(lm, dir); err != nil {
		t.Errorf("removing it twice: %v", err)
	}
	// A name that leads out of the folder removes nothing.
	outside := filepath.Join(t.TempDir(), "keep.gguf")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLanguageModel(LanguageModel{Name: "../" + filepath.Base(filepath.Dir(outside)) + "/keep.gguf"}, dir); err == nil {
		t.Error("a name leading out of the folder was taken")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("a file outside the folder went")
	}
	if err := RemoveSpeechModel(SpeechModel{Name: "."}, dir); err == nil {
		t.Error("the models folder itself was taken")
	}
}
