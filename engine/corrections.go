package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	i := sort.Search(len(t.HeardWords), func(i int) bool { return t.HeardWords[i].Start >= word.Start })
	put := putBack(t.HeardWords, i, text)
	trainingMu.Lock()
	corrections := LoadCorrections(logsDir)
	for j, says := range put {
		// A word that reads as the recogniser heard it again keeps no
		// correction, so a word put back is the word it was before.
		key := wordKey(t.HeardWords[j].Start)
		switch {
		case says == "":
			// A word removed keeps what it read, behind removedMark, so
			// the captions keep the room it took. Removed again, it keeps
			// what it read the first time.
			if was := t.HeardWords[j].Text; was != "" {
				corrections[key] = removedMark + was
			} else if _, kept := corrections[key]; !kept {
				corrections[key] = removedMark
			}
		case j < len(t.snapped) && says == strings.Join(strings.Fields(t.snapped[j].Text), " "):
			delete(corrections, key)
		default:
			corrections[key] = says
		}
	}
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

// putBack is what a correction of the i-th heard word changes, by heard
// word. A removed word is put back by typing it into the word beside it,
// "weil" made "weil ein", and that is the removed word coming back, so it
// goes back where it was heard, not into its neighbour: the words typed
// after what the word read go into the removed words that follow it, and
// the words typed before into the removed words before it. Without this
// the word took its time from its neighbour, and was lit while the
// neighbour was said and not while it was. Undo put it back where it was,
// and typing it back did not, though both are the same word in the same
// place.
//
// Each removed word takes one word, the nearest first. Words typed beyond
// the removed ones there are go to the farthest of them, which is the
// word they were typed beside.
func putBack(heard []Cue, i int, text string) map[int]string {
	put := map[int]string{i: text}
	was, now := strings.Fields(heard[i].Text), strings.Fields(text)
	if len(was) == 0 || len(now) <= len(was) {
		return put
	}
	removed := func(j int) bool { return j >= 0 && j < len(heard) && strings.TrimSpace(heard[j].Text) == "" }
	if slices.Equal(now[:len(was)], was) && removed(i+1) {
		extra := now[len(was):]
		var slots []int
		for j := i + 1; removed(j) && len(slots) < len(extra); j++ {
			slots = append(slots, j)
		}
		put[i] = strings.Join(was, " ")
		last := len(slots) - 1
		for k, j := range slots[:last] {
			put[j] = extra[k]
		}
		put[slots[last]] = strings.Join(extra[last:], " ")
		return put
	}
	if slices.Equal(now[len(now)-len(was):], was) && removed(i-1) {
		extra := now[:len(now)-len(was)]
		var slots []int
		for j := i - 1; removed(j) && len(slots) < len(extra); j-- {
			slots = append(slots, j)
		}
		put[i] = strings.Join(was, " ")
		// slots run back from the word, the nearest first.
		last := len(slots) - 1
		for k, j := range slots[:last] {
			put[j] = extra[len(extra)-1-k]
		}
		put[slots[last]] = strings.Join(extra[:len(extra)-last], " ")
	}
	return put
}
