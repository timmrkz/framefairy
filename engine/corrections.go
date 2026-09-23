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

// ApplyCorrections replaces the text of corrected words.
func ApplyCorrections(words []Cue, corrections map[string]string) {
	if len(corrections) == 0 {
		return
	}
	for i := range words {
		if text, ok := corrections[wordKey(words[i].Start)]; ok {
			words[i].Text = text
		}
	}
}

// SplitCorrected turns a word that was corrected into several words into one
// cue per word. The recogniser heard one word where more than one was said,
// so the span it measured is shared out by how long the words are. The audio
// is never read again for this: the highlight only has to run over the words
// inside the part where they were spoken.
func SplitCorrected(words []Cue) []Cue {
	out := make([]Cue, 0, len(words))
	for _, word := range words {
		parts := strings.Fields(word.Text)
		if len(parts) < 2 {
			out = append(out, word)
			continue
		}
		total := 0
		for _, part := range parts {
			total += utf8.RuneCountInString(part)
		}
		at, span := word.Start, word.End-word.Start
		for i, part := range parts {
			end := word.End
			if i < len(parts)-1 {
				end = at + span*float64(utf8.RuneCountInString(part))/float64(total)
			}
			out = append(out, Cue{at, end, part})
			at = end
		}
	}
	return out
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

// dropCaptionFile removes a clip's caption file, so the next render builds
// the captions again from the plan. A caption file left over from an
// earlier cut would otherwise be reused with the wrong timing.
func dropCaptionFile(planPath, clipID string) {
	_, clips, err := LoadClips(planPath)
	if err != nil {
		return
	}
	dir := filepath.Join(filepath.Dir(filepath.Dir(planPath)), "captions")
	for _, c := range clips {
		if c.ID != clipID {
			continue
		}
		if path, err := SafeChild(dir, c.Basename()+".srt"); err == nil {
			_ = os.Remove(path)
		}
	}
}

// SetWordText corrects one word of an episode. The correction is stored
// for the episode and applied to every clip of every clip set that contains
// the word. It returns how many clips changed.
func SetWordText(logsDir string, start float64, text string, t *Transcript) (int, error) {
	text, err := CleanWordText(text)
	if err != nil {
		return 0, err
	}
	var word *Cue
	for i := range t.Words {
		if math.Abs(t.Words[i].Start-start) < 0.0015 {
			word = &t.Words[i]
			break
		}
	}
	if word == nil {
		return 0, renderErr("there is no word at %s", HMS(start))
	}
	key := wordKey(word.Start)

	trainingMu.Lock()
	corrections := LoadCorrections(logsDir)
	corrections[key] = text
	body, err := marshalNoEscape(correctionsFile{Version: 1, Words: corrections})
	if err == nil {
		err = writeAtomic(correctionsPath(logsDir), append(body, '\n'))
	}
	trainingMu.Unlock()
	if err != nil {
		return 0, err
	}
	word.Text = text

	changed := 0
	for _, plan := range PlanSummaries(logsDir) {
		var ids []string
		err := editPlan(plan.Path, func(_ *object, clips []*object) error {
			for i, c := range clips {
				list, _ := c.values["words"].([]any)
				hit := false
				for _, item := range list {
					triple, ok := item.([]any)
					if ok && len(triple) == 3 && wordKey(number(triple[0])) == key {
						triple[2] = text
						hit = true
					}
				}
				if hit {
					fallback := twoDigits(i + 1)
					id := fallback
					if raw, ok := c.get("id"); ok {
						id = pyStr(raw)
					}
					ids = append(ids, SanitiseName(id, fallback))
				}
			}
			if len(ids) == 0 {
				return errNothingToEdit
			}
			return nil
		})
		if err == errNothingToEdit {
			continue
		}
		if err != nil {
			return changed, err
		}
		for _, id := range ids {
			dropCaptionFile(plan.Path, id)
		}
		changed += len(ids)
	}
	return changed, nil
}

var errNothingToEdit = renderErr("nothing to edit")
