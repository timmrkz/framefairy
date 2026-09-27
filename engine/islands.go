package engine

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// An island is a part of the episode transcribed out of turn, ahead of the
// transcription that runs from the start, so a clip can be made by hand
// where that transcription has not reached yet. It is kept as the
// transcript of a window, words-<from>-<to>.json, the same file the command
// line's --from and --to make, with the loudness its words are snapped to.
//
// Transcript adds an island's words wherever the whole-episode transcript
// has not reached. Once it has, its own words stand and the island is left
// unread: the minute or two of audio is heard again rather than stitched,
// so there is only ever one transcript to trust at any moment of the
// episode.

var islandNameRe = regexp.MustCompile(`^words-(\d+)-(\d+)\.json$`)

// IslandMargin is how much an island reaches past what a clip made by hand
// needs, on either side: room to find the sentence the playhead stands in,
// and room to drag the clip's edges further out afterwards.
const IslandMargin = 60.0

// IslandSeam is how far an island is heard past the edges of the part it
// is for, into what is transcribed already. A word the edge of a window
// cuts in two is heard whole on the other side of the seam, and the seam
// itself lies where both sides heard the audio with room around it.
const IslandSeam = 5.0

// islandFiles are the windows transcribed out of turn, in the order they
// lie in the episode.
func islandFiles(logsDir string) []string {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if islandNameRe.MatchString(entry.Name()) && entry.Type().IsRegular() {
			out = append(out, filepath.Join(logsDir, entry.Name()))
		}
	}
	// In time order, not in the order of their names, where 1000 comes
	// before 200.
	sort.SliceStable(out, func(a, b int) bool {
		wa, _ := islandWindow(out[a])
		wb, _ := islandWindow(out[b])
		if wa.Start != wb.Start {
			return wa.Start < wb.Start
		}
		return wa.End > wb.End
	})
	return out
}

func islandWindow(path string) (Window, bool) {
	m := islandNameRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return Window{}, false
	}
	from, err1 := strconv.Atoi(m[1])
	to, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil || to <= from || float64(to) > MaxEpisodeSeconds {
		return Window{}, false
	}
	return Window{float64(from), float64(to)}, true
}

// withIslands adds the words and the loudness of every island past covered,
// the point the whole-episode transcript has reached, to t.
//
// Each moment is read from one transcript only. Up to covered that is the
// transcript from the start. Past it, an island reads its own window, and
// where two islands overlap, the seam lies in the middle of the overlap,
// away from the edges where either of them may have cut a word in two. An
// island that lies inside another adds nothing.
func (p *Project) withIslands(t *Transcript, covered float64) *Transcript {
	stamp, err := stampOf(p.Source)
	if err != nil {
		return t
	}
	model := filepath.Base(filepath.Clean(p.modelDir()))
	type island struct {
		own Window
		t   *Transcript
	}
	var islands []island
	reached := covered
	for _, path := range islandFiles(p.LogsDir()) {
		w, ok := islandWindow(path)
		if !ok || w.End <= reached {
			continue
		}
		read, ok := readTranscript(path, stamp, model, w, p.Base.SilenceDB)
		if !ok {
			continue
		}
		own := Window{math.Max(w.Start, covered), w.End}
		if n := len(islands); n > 0 && w.Start < islands[n-1].own.End {
			seam := (math.Max(w.Start, covered) + islands[n-1].own.End) / 2
			islands[n-1].own.End = seam
			own.Start = seam
		}
		islands = append(islands, island{own, read})
		reached = w.End
	}
	var words []Cue
	var levels []Reading
	for _, is := range islands {
		for _, word := range is.t.Words {
			if mid := (word.Start + word.End) / 2; mid >= is.own.Start && mid < is.own.End &&
				word.Start >= covered {
				words = append(words, word)
			}
		}
		for _, r := range is.t.Levels() {
			if r.At >= is.own.Start && r.At < is.own.End {
				levels = append(levels, r)
			}
		}
	}
	if len(words) == 0 {
		return t
	}
	if t == nil {
		t = &Transcript{}
	}
	all := append(append([]Cue(nil), t.Words...), words...)
	sort.SliceStable(all, func(a, b int) bool { return all[a].Start < all[b].Start })
	t.Words = all
	t.Extra = append(t.Extra, levels...)
	return t
}

func (p *Project) modelDir() string {
	if p.Base.ASRModel != "" {
		return p.Base.ASRModel
	}
	return DefaultModelDir()
}

// Heard says whether the transcript, islands included, has words around a
// moment, enough to make a clip there by hand.
func (p *Project) Heard(at float64) bool {
	t, err := p.Transcript()
	if err != nil {
		return false
	}
	for _, w := range t.Words {
		if w.End >= at-5 && w.Start <= at+5 {
			return true
		}
	}
	return false
}

// HearAround transcribes the part of the episode a clip made by hand at a
// moment needs, when the transcription from the start has not reached it:
// the clip's longest length on the side it grows to, and IslandMargin on
// both sides. What is transcribed already, by that transcription or by an
// island, is not heard again: only the gaps are, each a new island
// reaching IslandSeam into what is there on either side.
func (p *Project) HearAround(ctx context.Context, at float64, backward bool, duration float64) error {
	for _, gap := range p.unheard(at, backward, duration) {
		w := Window{math.Max(0, math.Floor(gap.Start-IslandSeam)), math.Ceil(gap.End + IslandSeam)}
		if duration > 0 {
			w.End = math.Min(w.End, math.Floor(duration))
		}
		if _, err := p.engine.LoadTranscript(ctx, p.Source, w, p.LogsDir(), p.modelDir(),
			p.Base.SilenceDB, true); err != nil {
			return err
		}
	}
	return nil
}

// Unheard says whether HearAround has anything to hear for a clip made by
// hand at a moment, so the app can say it is transcribing only when it is.
func (p *Project) Unheard(at float64, backward bool, duration float64) bool {
	return len(p.unheard(at, backward, duration)) > 0
}

// unheard is what of the part a clip made by hand at a moment needs is
// not transcribed yet, in the order it lies in the episode. A gap shorter
// than a second is left, a word does not fit in it.
func (p *Project) unheard(at float64, backward bool, duration float64) []Window {
	from, to := at-IslandMargin, at+p.Base.Max+IslandMargin
	if backward {
		from, to = at-p.Base.Max-IslandMargin, at+IslandMargin
	}
	from = math.Max(0, from)
	if duration > 0 {
		to = math.Min(to, duration)
	}
	heard := append([]Window{{0, p.transcribedTo()}}, p.islands()...)
	var gaps []Window
	for _, w := range heard {
		if w.End <= from || w.Start >= to {
			continue
		}
		if w.Start > from {
			gaps = append(gaps, Window{from, w.Start})
		}
		from = math.Max(from, w.End)
	}
	if from < to {
		gaps = append(gaps, Window{from, to})
	}
	var out []Window
	for _, g := range gaps {
		if g.End-g.Start >= 1 {
			out = append(out, g)
		}
	}
	return out
}

// islands are the windows of the islands that can be read, for this
// episode and this speech model, in time order. One that cannot, from an
// older model say, is heard again.
func (p *Project) islands() []Window {
	stamp, err := stampOf(p.Source)
	if err != nil {
		return nil
	}
	model := filepath.Base(filepath.Clean(p.modelDir()))
	var out []Window
	for _, path := range islandFiles(p.LogsDir()) {
		if w, ok := islandWindow(path); ok {
			if _, ok := readTranscript(path, stamp, model, w, p.Base.SilenceDB); ok {
				out = append(out, w)
			}
		}
	}
	return out
}

// transcribedTo is how far the transcription from the start has come.
func (p *Project) transcribedTo() float64 {
	stamp, err := stampOf(p.Source)
	if err != nil {
		return 0
	}
	file, _, _, ok := readTranscriptFile(filepath.Join(p.LogsDir(), TranscriptName(nil)), stamp,
		filepath.Base(filepath.Clean(p.modelDir())))
	if !ok {
		return 0
	}
	return float64(file.To)
}
