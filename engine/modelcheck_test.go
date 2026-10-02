package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A model the app downloaded is loaded only while it is the file its
// SHA-256 says: checked once, not read again while it stays as it was,
// and read again, and refused, the moment it is not.
func TestAModelIsCheckedBeforeItIsLoaded(t *testing.T) {
	dir := t.TempDir()
	body := ggufBytes(4096)
	m := LanguageModel{Name: "test.gguf", Title: "Test", SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
	path := filepath.Join(dir, m.Name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkModel(path, dir, m); err != nil {
		t.Fatalf("the right model was refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, checkedModels)); err != nil {
		t.Fatalf("what the check found was not kept: %v", err)
	}

	// Other bytes, a moment later, the way a file changed by anything
	// else would be left.
	changed := append([]byte(nil), body...)
	changed[len(changed)-1] ^= 0xff
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, later, later)
	err := checkModel(path, dir, m)
	if err == nil {
		t.Fatal("a model that changed after it was checked was loaded")
	}
	if !strings.Contains(err.Error(), "not the file it was") {
		t.Errorf("refused for another reason: %v", err)
	}
}

// A model chosen by hand, a file the catalogue does not know, is the
// person's own and is loaded as it is. So is one outside the models folder.
func TestAModelChosenByHandIsNotChecked(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "my-own.gguf")
	if err := os.WriteFile(own, ggufBytes(64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModelFile(own, dir); err != nil {
		t.Errorf("a model of the person's own was refused: %v", err)
	}
	elsewhere := filepath.Join(t.TempDir(), LanguageModels()[0].Name)
	if err := os.WriteFile(elsewhere, ggufBytes(64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModelFile(elsewhere, dir); err != nil {
		t.Errorf("a file outside the models folder was held to the catalogue: %v", err)
	}
	inside := filepath.Join(dir, LanguageModels()[0].Name)
	if err := os.WriteFile(inside, ggufBytes(64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckModelFile(inside, dir); err == nil {
		t.Error("a file named like a known model, and not that model, was loaded")
	}
}
