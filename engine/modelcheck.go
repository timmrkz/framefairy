package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A language model the app downloaded is checked again before it is loaded.
// Its SHA-256 is pinned beside it, see LanguageModels, and checked when it
// arrives, but the file then lies in the models folder for months, and a
// model is fifteen gigabytes that llama-server reads with no questions. So
// a model of a name the catalogue knows, in the models folder, is loaded
// only once it is the file that sum says.
//
// Reading fifteen gigabytes takes several seconds, so it is not read before
// every search: what a check found is kept beside the models, with the size
// and the time of the file then, and the file is read again only when
// either has moved. A model somebody chose by hand, a file the catalogue
// does not know, is theirs and is loaded as it is.

// checkedModels is the file in the models folder that keeps what the
// checks found.
const checkedModels = ".checked.json"

type modelCheck struct {
	Size   int64     `json:"size"`
	Mod    time.Time `json:"mod"`
	SHA256 string    `json:"sha256"`
}

var modelChecks sync.Mutex

// CheckModelFile says whether the model at path may be loaded: a model the
// catalogue knows, in dir, has to be the file its SHA-256 says. dir is the
// models folder, ModelsDir when empty.
func CheckModelFile(path, dir string) error {
	if dir == "" {
		dir = ModelsDir()
	}
	known, ok := LanguageModelByName(filepath.Base(path))
	if !ok || known.SHA256 == "" || filepath.Clean(filepath.Dir(path)) != filepath.Clean(dir) {
		return nil
	}
	return checkModel(path, dir, known)
}

// checkModel is CheckModelFile for a model already known to be known, split
// out so a test can hand it a model of its own.
func checkModel(path, dir string, known LanguageModel) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	modelChecks.Lock()
	defer modelChecks.Unlock()
	record := filepath.Join(dir, checkedModels)
	checks := map[string]modelCheck{}
	if body, err := os.ReadFile(record); err == nil {
		_ = json.Unmarshal(body, &checks)
	}
	was, seen := checks[known.Name]
	if seen && was.Size == info.Size() && was.Mod.Equal(info.ModTime()) && strings.EqualFold(was.SHA256, known.SHA256) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	sum := sha256.New()
	_, err = io.Copy(sum, file)
	file.Close()
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, known.SHA256) {
		return renderErr("%s is not the file it was when it was downloaded, so it is not loaded. "+
			"Remove it and download it again.", known.Title)
	}
	noteModelChecked(record, checks, known, info)
	return nil
}

// noteModelChecked keeps what a check of a model found, in checks as read
// from record. A record that cannot be written only means the next check
// reads the model again.
func noteModelChecked(record string, checks map[string]modelCheck, m LanguageModel, info os.FileInfo) {
	checks[m.Name] = modelCheck{Size: info.Size(), Mod: info.ModTime(), SHA256: m.SHA256}
	if body, err := json.MarshalIndent(checks, "", "  "); err == nil {
		_ = writeAtomic(record, body)
	}
}

// modelArrived keeps what the download of a model has just checked, so the
// first search after it does not read the whole file a second time.
func modelArrived(dir string, m LanguageModel, path string) {
	info, err := os.Stat(path)
	if err != nil || m.SHA256 == "" {
		return
	}
	modelChecks.Lock()
	defer modelChecks.Unlock()
	record := filepath.Join(dir, checkedModels)
	checks := map[string]modelCheck{}
	if body, err := os.ReadFile(record); err == nil {
		_ = json.Unmarshal(body, &checks)
	}
	noteModelChecked(record, checks, m, info)
}
