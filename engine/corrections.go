package engine

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/afero"
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
func LoadCorrections(fsys afero.Fs, logsDir string) map[string]string {
	var file correctionsFile
	data, err := afero.ReadFile(fsys, correctionsPath(logsDir))
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
func SetWordText(fsys afero.Fs, logsDir string, at float64, text string, t *Transcript) error {
	return changeWords(fsys, logsDir, t, func(corrections map[string]string, work *Transcript) error {
		return correctWord(corrections, at, text, work)
	})
}

// changeWords makes a change to the corrections of an episode, worked out
// against a copy of its words, writes them, and then corrects t. Nothing is
// written when the change fails or changes nothing.
func changeWords(fsys afero.Fs, logsDir string, t *Transcript,
	change func(corrections map[string]string, work *Transcript) error) error {
	trainingMu.Lock()
	corrections := LoadCorrections(fsys, logsDir)
	was := maps.Clone(corrections)
	work := *t
	err := change(corrections, &work)
	if err == nil && !maps.Equal(was, corrections) {
		var body []byte
		body, err = marshalNoEscape(correctionsFile{Version: 1, Words: corrections})
		if err == nil {
			err = WriteAtomic(fsys, correctionsPath(logsDir), append(body, '\n'))
		}
	}
	trainingMu.Unlock()
	if err != nil {
		return err
	}
	t.Correct(corrections)
	return nil
}

// correctWord keeps in corrections what the word heard at a moment says,
// the way SetWordText keeps it, and corrects t from them, so the next word
// is changed against this one.
func correctWord(corrections map[string]string, at float64, text string, t *Transcript) error {
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
	for j, says := range putBack(t.HeardWords, i, text) {
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

// RemoveCaption removes the caption of a clip that begins on a word, the
// one heard at first: every word in it is removed the way SetWordText
// removes one, so each is still the word heard there, to put back on its
// own or with an undo. A word that a correction made several is removed
// only where it is in this caption. What is left is laid out as it was:
// the caption goes and leaves its time empty, and the one before it goes
// when it did, see Captions.
func RemoveCaption(fsys afero.Fs, logsDir, planPath, clipID string, first float64, t *Transcript, look *Look) error {
	view, err := ClipCaptionsView(fsys, planPath, clipID, t, look)
	if err != nil {
		return err
	}
	var cue *CaptionView
	for i := range view.Captions {
		if math.Abs(view.Captions[i].First-first) < 1e-6 && len(view.Captions[i].Lines) > 0 {
			cue = &view.Captions[i]
			break
		}
	}
	if cue == nil {
		return renderErr("there is no caption at %s", HMS(first))
	}
	// The heard words of the caption, each with the parts of it the
	// caption holds, or all of it. A word the captions draw in halves is
	// there twice, and is one word.
	whole := map[float64]string{}
	parts := map[float64]map[int]bool{}
	var order []float64
	for _, line := range cue.Lines {
		for _, w := range line.Words {
			if w.Said == nil {
				continue
			}
			at := *w.Said
			if _, seen := whole[at]; !seen {
				whole[at] = w.Whole
				parts[at] = map[int]bool{}
				order = append(order, at)
			}
			if w.Part == nil {
				parts[at] = nil
			} else if parts[at] != nil {
				parts[at][*w.Part] = true
			}
		}
	}
	if len(order) == 0 {
		return renderErr("the caption at %s has no words to remove", HMS(first))
	}
	return changeWords(fsys, logsDir, t, func(corrections map[string]string, work *Transcript) error {
		return removeParts(corrections, work, order, whole, parts)
	})
}

// removeParts removes heard words, each started at a moment of order, the
// way SetWordText removes one, in corrections and in t. whole is what each
// reads, and parts the words of it a correction made several of that go,
// or nil when all of it goes.
func removeParts(corrections map[string]string, t *Transcript, order []float64, whole map[float64]string,
	parts map[float64]map[int]bool) error {
	for _, at := range order {
		var kept []string
		if parts[at] != nil {
			for i, word := range strings.Fields(whole[at]) {
				if !parts[at][i] {
					kept = append(kept, word)
				}
			}
		}
		if err := correctWord(corrections, at, strings.Join(kept, " "), t); err != nil {
			return err
		}
	}
	return nil
}

// trimCaption is what dragging an edge of a caption across its words does
// to them. Dragged in, the words the edge passes leave the caption, removed
// the way SetWordText removes one, so pulling the left edge of "one two
// three" past "one" reads "two three", and the right edge past "three"
// reads "one two". Dragged back out, the words of the caption removed
// before that it passes come back. A word is passed when the edge crosses
// its middle, on the clip's clock, which is where the word is drawn: a
// word the start of the clip cuts into is passed in the part of it the
// clip shows. A caption keeps at least one word this way, because delete
// removes a caption whole.
//
// to is where the edge is, on the clip's clock. The caption is the one
// laid out now, with the edge where it stands. The words are changed in
// corrections and in t, and nothing is written. It says how many words it
// took out and put back.
func trimCaption(corrections map[string]string, clip Clip, c LaidCaption, edge string, to float64,
	t *Transcript) (CaptionEdit, error) {
	if len(clip.Segments) == 0 {
		return CaptionEdit{}, nil
	}
	from := c.Start
	if edge == "end" {
		from = c.End
	}
	low, high := math.Min(from, to), math.Max(from, to)
	in := (edge == "start") == (to > from)
	if !in && len(c.Words) > 0 {
		// An edge whose word was taken out may stand past the middle of
		// that word already. Out from there, the words past the words
		// shown are what it crosses.
		if edge == "end" {
			low = math.Min(low, c.Words[len(c.Words)-1].End)
		} else {
			high = math.Max(high, c.Words[0].Start)
		}
	}
	// The words the caption was laid out from, the removed ones among them,
	// on the clip's clock and on the episode's.
	all, was := clipWords(clip, t.captionWordsIn(clip.Segments[0].Start-captionReach,
		clip.Segments[len(clip.Segments)-1].End+captionReach))
	length := clipLength(clip)
	var gone, back []Cue
	shown := 0
	for k, w := range was {
		if w.Start < c.Opens-0.0005 || w.Start > c.Closes+0.0005 {
			continue
		}
		text, removed := removedText(w.Text)
		if !removed {
			shown++
		}
		mid := shownMiddle(all[k], length)
		if mid <= low || mid >= high {
			continue
		}
		switch {
		case in && !removed:
			gone = append(gone, w)
		case !in && removed && text != "":
			back = append(back, Cue{w.Start, w.End, text})
		}
	}
	if len(gone) > 0 && len(gone) == shown {
		// The word at the other edge stays.
		if edge == "end" {
			gone = gone[1:]
		} else {
			gone = gone[:len(gone)-1]
		}
	}
	for _, w := range back {
		if err := correctWord(corrections, w.Start, w.Text, t); err != nil {
			return CaptionEdit{}, err
		}
	}
	whole := map[float64]string{}
	parts := map[float64]map[int]bool{}
	var order []float64
	for _, w := range gone {
		heard, ok := t.HeardAt(w.Start)
		if !ok {
			continue
		}
		at := heard.Start
		if _, seen := whole[at]; !seen {
			whole[at] = heard.Text
			parts[at] = map[int]bool{}
			order = append(order, at)
		}
		if len(strings.Fields(heard.Text)) < 2 {
			parts[at] = nil
		} else if parts[at] != nil {
			parts[at][t.PartOf(heard, w)] = true
		}
	}
	done := CaptionEdit{Removed: len(gone), Back: len(back)}
	return done, removeParts(corrections, t, order, whole, parts)
}
