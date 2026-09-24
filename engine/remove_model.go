package engine

import (
	"errors"
	"io/fs"
	"os"
)

// Taking a model off the machine again. A model is gigabytes, and a person
// who tried one and went back to another should get the room back without
// finding the folder by hand.

// RemoveLanguageModel deletes an installed model and whatever a download of
// it left behind, so nothing of it is left on the disk.
func RemoveLanguageModel(m LanguageModel, dir string) error {
	if dir == "" {
		dir = ModelsDir()
	}
	return removeAll(dir, m.Name, m.Name+".part")
}

// RemoveSpeechModel deletes an installed speech model, its folder, and
// whatever a download or an unpacking of it left behind.
func RemoveSpeechModel(m SpeechModel, dir string) error {
	if dir == "" {
		dir = ModelsDir()
	}
	return removeAll(dir, m.Name, m.Name+".part", m.Name+".unpacking")
}

// removeAll deletes each name inside dir. A name is only ever taken inside
// dir, judged by where it really leads, so a name from the catalogue that
// somehow pointed elsewhere deletes nothing. What is already gone is fine.
func removeAll(dir string, names ...string) error {
	for _, name := range names {
		path, err := SafeChild(dir, name)
		if err != nil {
			return err
		}
		if path == resolvePath(dir) {
			return renderErr("refusing to remove the models folder itself")
		}
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
