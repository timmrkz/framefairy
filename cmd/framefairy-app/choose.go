package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// The clip the app chooses by itself. A hand chooses with ChooseClip, and
// everything else that chooses a clip is here, in the app's Go side, so
// the workspace decides nothing about it: it draws the clip the video's
// state says is chosen, see VideoState.Chosen, and sends what the hand
// did, a pick and whether the video plays. See the decision record
// docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.
//
// The app chooses a clip
//
//  1. when a search writes its first clip, the earliest it has written,
//     if the clip chosen is still the one chosen when it began,
//  2. when a search ends done, the earliest of what it found, if the clip
//     chosen is still the one chosen when it began or the one it chose.
//     A window searched again comes back under the names its clips had,
//     so then it is the first clip of its clip set,
//  3. never when a search stopped, was cancelled or failed,
//  4. when the clip chosen is not among the clips, the first, so a video
//     opens on the clip chosen last if it is still there, or else the
//     first,
//  5. when the clip chosen is removed, the one beside it, and when it is
//     put back while that one is still chosen, the clip put back,
//  6. after an undo or a redo, the clip it changed,
//  7. when a clip on its way that is chosen, a card, is written, the clip.
//
// The first two only while the video is paused, so a picture that is
// playing never jumps to a clip nobody asked for.

// followed is the search the app follows to choose its clips: the clip
// chosen and the clips there were when it began, and the clip it chose.
type followed struct {
	id     string
	before string
	known  map[string]bool
	chose  string
	shown  bool
}

// Playing says whether the video plays in its workspace. The app chooses
// a search's clips only while it is paused.
func (s *FrameFairy) Playing(path string, playing bool) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	p := s.chosenIn(path)
	p.load(s.store.files, path)
	p.mu.Lock()
	p.playing = playing
	p.mu.Unlock()
	return nil
}

// searchBegins notes the clips there are as a search of the video begins
// its work, before it writes any, so the clips it writes are known as its
// own however late a state first sees it running, rules 1 and 2.
func (s *FrameFairy) searchBegins(path string) {
	known := map[string]bool{}
	for _, summary := range s.plansOf(path) {
		for _, c := range summary.View().Clips {
			known[filepath.Base(summary.Path)+"/"+c.ID] = true
		}
	}
	p := s.chosenIn(path)
	p.load(s.store.files, path)
	p.mu.Lock()
	p.begins = &followed{before: p.kept.Clip, known: known}
	p.mu.Unlock()
}

// chosenFor is the clip chosen in the video whose state st is, read with
// its jobs and its clips, after the app chose by rules 1, 2, 3, 4 and 7
// from them, and the card that clip was while it was on its way, if it
// was one. A state older than the newest it chose from chooses nothing.
// It says what is chosen now when its clips hold it, and otherwise the
// clip chosen before, so a state never names a clip it does not hold.
func (s *FrameFairy) chosenFor(path string, st VideoState) (clip, was string) {
	p := s.chosenIn(path)
	var said []string
	if _, err := p.with(s.store.files, path, func(p *picked) bool {
		defer func() { clip, was = p.answer(st) }()
		if st.Rev != 0 && st.Rev <= p.decided {
			return false
		}
		p.decided = st.Rev
		before := p.kept.Clip
		now := before
		if landed := landedClip(st, now); landed != "" {
			p.card, p.became = now, landed
			now = landed
			said = append(said, "the clip made by hand was written")
		}
		now, said = p.follow(st, now, said)
		if now != before {
			p.prior, p.kept.Clip = before, now
			return true
		}
		if first := firstKept(st.Clips); first != "" && !holdsClip(st.Clips, now) && card(st.Jobs, now) == nil {
			// Rule 4. What is chosen here is not written: the file says
			// what a hand or a search chose, and a video opened again
			// opens on the clip chosen last once it is there.
			p.prior, p.kept.Clip = before, first
			said = append(said, "the clip chosen is not there, so the first")
		}
		return false
	}); err != nil {
		theLog.line(zerolog.WarnLevel, "chosen").Str("video", filepath.Base(path)).Err(err).Msg("the clip the app chose could not be kept")
	}
	for _, why := range said {
		theLog.line(zerolog.InfoLevel, "chosen").Str("video", filepath.Base(path)).Str("clip", clip).Str("why", why).Msg("the app chose a clip")
	}
	return clip, was
}

// answer is the clip chosen a state says, and the card it was, with the
// lock held: the clip chosen, or for a state older than the newest that
// chose, the clip chosen before when its clips do not hold the one now,
// and for a card chosen whose clip the state holds, that clip.
func (p *picked) answer(st VideoState) (string, string) {
	now := p.kept.Clip
	if st.Rev != 0 && st.Rev < p.decided && !holdsClip(st.Clips, now) && card(st.Jobs, now) == nil {
		now = ""
		if holdsClip(st.Clips, p.prior) {
			now = p.prior
		}
	}
	if now != "" && now == p.became {
		return now, p.card
	}
	// A card whose clip this state holds is that clip, rule 7, whatever
	// the state was read before: a hand picks a card its window still
	// shows after its clip was written, and a state answered after the
	// pick said the card was chosen, which a list that holds its clip
	// leaves out, so for a frame nothing was chosen, issue #697.
	if clip := landedClip(st, now); clip != "" {
		return clip, now
	}
	return now, ""
}

// follow chooses for the newest search of the video, rules 1, 2 and 3,
// with the lock held. now is the clip chosen.
func (p *picked) follow(st VideoState, now string, said []string) (string, []string) {
	var search *Job
	for i := range st.Jobs {
		if st.Jobs[i].Kind == engine.JobSearch {
			search = &st.Jobs[i]
		}
	}
	if search == nil {
		p.search = nil
		return now, said
	}
	if p.search == nil || p.search.id != search.ID {
		switch {
		case p.begins != nil:
			// The clips there were as the search began its work.
			p.search, p.begins = p.begins, nil
			p.search.id = search.ID
		case search.going():
			// A search seen running that began before the app noted it,
			// one read back as the app starts.
			known := map[string]bool{}
			for _, c := range st.Clips {
				known[c.Key] = true
			}
			p.search = &followed{id: search.ID, before: now, known: known}
		}
	}
	f := p.search
	if f == nil || f.id != search.ID {
		return now, said
	}
	var found []ClipEntry
	for _, c := range st.Clips {
		if !f.known[c.Key] {
			found = append(found, c)
		}
	}
	if search.going() {
		if len(found) == 0 || f.shown {
			return now, said
		}
		f.shown = true
		if now != f.before || p.playing {
			return now, said
		}
		f.chose = found[0].Key
		return f.chose, append(said, "the first clip the search found")
	}
	p.search = nil
	if search.State != JobDone {
		return now, said
	}
	if len(found) > 0 {
		f.shown = true
		if (now != f.before && now != f.chose) || p.playing || found[0].Key == now {
			return now, said
		}
		return found[0].Key, append(said, "the earliest clip the search found")
	}
	if f.shown || now != f.before || p.playing || search.Result == "" {
		return now, said
	}
	for _, c := range st.Clips {
		if c.Plan == search.Result {
			return c.Key, append(said, "the first clip of the window searched again")
		}
	}
	return now, said
}

// firstKept is the first of the clips not removed, or nothing.
func firstKept(clips []ClipEntry) string {
	for _, c := range clips {
		if !c.Rejected {
			return c.Key
		}
	}
	return ""
}

// holdsClip says whether key is one of the clips.
func holdsClip(clips []ClipEntry, key string) bool {
	for _, c := range clips {
		if c.Key == key {
			return true
		}
	}
	return false
}

// cardKey is the key of a clip on its way, a card: its job's id and the
// clip's number, the key the workspace knows the card by, see
// lib/arriving.ts.
func cardKey(job string, n int) string {
	return job + "/" + strconv.Itoa(n)
}

// card is the job whose clip on its way key is, the job's id and the
// clip's number, or nil when key is no card.
func card(jobs []Job, key string) *Job {
	id, n, ok := strings.Cut(key, "/")
	if !ok {
		return nil
	}
	if _, err := strconv.Atoi(n); err != nil {
		return nil
	}
	for i := range jobs {
		if jobs[i].ID == id {
			return &jobs[i]
		}
	}
	return nil
}

// landedClip is the clip a chosen card was written as, once it is in the
// clips, rule 7, or nothing. Its job says the key from the moment the
// clip has one, and its result once it is done.
func landedClip(st VideoState, key string) string {
	j := card(st.Jobs, key)
	if j == nil {
		return ""
	}
	_, n, _ := strings.Cut(key, "/")
	for _, u := range j.Underway {
		if strconv.Itoa(u.N) == n && u.Clip != "" && holdsClip(st.Clips, u.Clip) {
			return u.Clip
		}
	}
	if j.Kind == engine.JobClip && j.State == JobDone && holdsClip(st.Clips, j.Result) {
		return j.Result
	}
	return ""
}

// removing chooses for a clip removed or put back, rule 5. before is the
// state as it was before the remove, which the clip beside is read from:
// the clips not removed, in the order of the list, the same the arrow
// keys walk.
func (s *FrameFairy) removing(path, key string, removed bool, before VideoState) {
	p := s.chosenIn(path)
	var why string
	if _, err := p.with(s.store.files, path, func(p *picked) bool {
		if removed {
			if p.kept.Clip != key {
				return false
			}
			var list []ClipEntry
			for _, c := range before.Clips {
				if !c.Rejected || c.Key == key {
					list = append(list, c)
				}
			}
			beside := ""
			for i, c := range list {
				if c.Key != key {
					continue
				}
				if i+1 < len(list) {
					beside = list[i+1].Key
				} else if i > 0 {
					beside = list[i-1].Key
				}
			}
			if p.instead == nil {
				p.instead = map[string]string{}
			}
			p.instead[key] = beside
			p.kept.Clip = beside
			why = "the clip beside the one removed"
			return true
		}
		then, ok := p.instead[key]
		delete(p.instead, key)
		if !ok || p.kept.Clip != then {
			return false
		}
		p.kept.Clip = key
		why = "the clip put back, which was chosen when it was removed"
		return true
	}); err != nil {
		theLog.line(zerolog.WarnLevel, "chosen").Str("video", filepath.Base(path)).Err(err).Msg("the clip the app chose could not be kept")
	}
	if why != "" {
		theLog.line(zerolog.InfoLevel, "chosen").Str("video", filepath.Base(path)).Str("clip", p.get(s.store.files, path).Clip).Str("why", why).Msg("the app chose a clip")
	}
}

// undone chooses the clip an undo or a redo changed, rule 6.
func (s *FrameFairy) undone(path, key string) {
	if key == "" {
		return
	}
	changed, err := s.chosenIn(path).change(s.store.files, path, func(c *chosen) bool {
		if c.Clip == key {
			return false
		}
		c.Clip = key
		return true
	})
	if err != nil {
		theLog.line(zerolog.WarnLevel, "chosen").Str("video", filepath.Base(path)).Err(err).Msg("the clip the app chose could not be kept")
	}
	if changed {
		theLog.line(zerolog.InfoLevel, "chosen").Str("video", filepath.Base(path)).Str("clip", key).Str("why", "the clip the undo changed").Msg("the app chose a clip")
	}
}

// stateBefore is the state of a video read before an edit that chooses
// from it, or an empty one when it cannot be read.
func (s *FrameFairy) stateBefore(path string) VideoState {
	st, err := s.State(context.Background(), path)
	if err != nil && !errors.Is(err, errNotInLibrary) {
		theLog.line(zerolog.WarnLevel, "chosen").Str("video", filepath.Base(path)).Err(err).Msg("the clips could not be read before the edit")
	}
	return st
}
