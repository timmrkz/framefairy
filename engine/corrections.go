package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Word corrections are kept per episode in logs/corrections.json, keyed by
// the millisecond a word starts. They change what captions show and never
// the stored transcript, so the model's prompts stay the same and the
// recogniser's own output is never lost.

type correctionsFile struct {
	Version int               `json:"version"`
	Words   map[string]string `json:"words"`
}

func correctionsPath(logsDir string) string {
	return filepath.Join(logsDir, "corrections.json")
}

func wordKey(start float64) string {
	return fmt.Sprintf("%d", int64(math.Round(start*1000)))
}

// LoadCorrections reads an episode's word corrections.
func LoadCorrections(logsDir string) map[string]string {
	var file correctionsFile
	data, err := os.ReadFile(correctionsPath(logsDir))
	if err != nil || json.Unmarshal(data, &file) != nil || file.Words == nil {
		return map[string]string{}
	}
	return file.Words
}

// CleanWordText checks a corrected word.
func CleanWordText(text string) (string, error) {
	text = strings.Join(strings.Fields(Scrub(text, 200)), " ")
	if text == "" {
		return "", renderErr("a word cannot be empty")
	}
	if utf8.RuneCountInString(text) > 60 {
		return "", renderErr("a word can be at most 60 characters")
	}
	return text, nil
}

// SetWordText corrects one word of an episode, the word heard at a moment.
// The correction is kept for the episode, against the word the recogniser
// heard, and the words are made again with it, see Transcript.Correct. No
// clip keeps words of its own, so every clip that says the word says it
// corrected from here on.
//
// A word corrected to nothing is removed: it is kept as an empty
// correction, so the word heard is still there to correct again and an
// undo puts it back, and the words leave it out. Removing a word was
// lost when the words became one list, because an empty word was refused.
func SetWordText(logsDir string, at float64, text string, t *Transcript) error {
	if strings.TrimSpace(Scrub(text, 200)) == "" {
		text = ""
	} else {
		var err error
		if text, err = CleanWordText(text); err != nil {
			return err
		}
	}
	word, ok := t.HeardAt(at)
	if !ok {
		return renderErr("there is no word at %s", HMS(at))
	}
	trainingMu.Lock()
	corrections := LoadCorrections(logsDir)
	corrections[wordKey(word.Start)] = text
	body, err := marshalNoEscape(correctionsFile{Version: 1, Words: corrections})
	if err == nil {
		err = writeAtomic(correctionsPath(logsDir), append(body, '\n'))
	}
	trainingMu.Unlock()
	if err != nil {
		return err
	}
	t.Correct(corrections)
	return nil
}
