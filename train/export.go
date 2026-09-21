// Package train turns the decisions recorded next to each episode into
// training files for the clip selection model. It is a tool for improving
// the model and is not part of the app.
package train

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"framefairy/engine"
)

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sftRecord struct {
	Messages []Message      `json:"messages"`
	Meta     map[string]any `json:"meta"`
}

type dpoRecord struct {
	Prompt   []Message      `json:"prompt"`
	Chosen   []Message      `json:"chosen"`
	Rejected []Message      `json:"rejected"`
	Meta     map[string]any `json:"meta"`
}

type answerClip struct {
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Reason string   `json:"reason"`
	Keep   [][2]int `json:"keep"`
}

// Manifest describes an export.
type Manifest struct {
	Created       string              `json:"created"`
	Engine        string              `json:"engine"`
	Schema        int                 `json:"schema"`
	PromptVersion []int               `json:"prompt_versions"`
	EvalShare     float64             `json:"eval_share"`
	Counts        map[string]int      `json:"counts"`
	Episodes      map[string][]string `json:"episodes"`
}

// Episode is a folder of training records. There is one folder for every
// episode now, and every record in it says which episode it came from, so
// this is usually a single entry. Older versions kept a folder beside each
// episode, and those are still read.
type Episode struct {
	Name string
	Dir  string
}

// FindEpisodes looks for training records: the folder itself, and any
// training folder of an episode below it.
func FindEpisodes(root string) ([]Episode, error) {
	var out []Episode
	if _, err := os.Stat(filepath.Join(root, "plans.jsonl")); err == nil {
		out = append(out, Episode{Name: filepath.Base(root), Dir: root})
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "training" && strings.HasSuffix(filepath.Dir(path), ".framefairy") {
			if _, err := os.Stat(filepath.Join(path, "plans.jsonl")); err == nil {
				name := strings.TrimSuffix(filepath.Base(filepath.Dir(path)), ".framefairy")
				out = append(out, Episode{Name: name, Dir: path})
			}
			return fs.SkipDir
		}
		return nil
	})
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, err
}

func readLines(path string, fn func([]byte)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		if json.Valid(scanner.Bytes()) {
			fn(append([]byte(nil), scanner.Bytes()...))
		}
	}
}

// isEval puts an episode in the held-out split by a stable hash of its
// name, so the split never changes between exports and never cuts an
// episode in two.
func isEval(name string, share float64) bool {
	sum := sha256.Sum256([]byte(name))
	return float64(binary.BigEndian.Uint64(sum[:8]))/float64(^uint64(0)) < share
}

// candidateState is everything decided about one proposed clip. The same
// clip proposed again for the same prompt shares one state, so doing the
// same thing twice counts once.
type candidateState struct {
	cand      engine.RecordCandidate
	rejected  bool
	decided   bool
	published bool
	rendered  bool
	changed   bool
	viewed    bool
	edited    bool
	final     *engine.FinalRecord
	changes   *engine.Changes
	reasons   []string
	order     int
}

// Weights of the outcomes, written next to each record so a training run
// can use them or not.
const (
	weightPassedOver = 0.5
	weightKept       = 1.0
	weightRendered   = 2.0
	weightPublished  = 3.0
)

type example struct {
	key     string
	system  string
	prompt  string
	episode string
	version int
	states  map[string]*candidateState
	order   []string
}

func candidateKey(c engine.RecordCandidate) string {
	return fmt.Sprint(c.Keep)
}

// Export writes train/ and eval/ with sft.jsonl and dpo.jsonl, and a
// manifest.
//
// Plans that asked the model the same thing are one example. Within it, a
// clip is identified by the lines it proposed, and only the latest state of
// each clip counts. The outcome of a clip is, from strongest to weakest:
// published, rendered, kept or edited, rejected. A rendered clip that nobody
// changed is a full hit. A changed clip gives the changed version as the
// target and, for preference training, the pair of changed over proposed.
func Export(episodes []Episode, out string, evalShare float64) (Manifest, error) {
	m := Manifest{Created: time.Now().UTC().Format(time.RFC3339), Engine: engine.Version,
		Schema: engine.TrainingSchema, EvalShare: evalShare, Counts: map[string]int{},
		Episodes: map[string][]string{"train": {}, "eval": {}}}
	files := map[string]*os.File{}
	open := func(split, name string) (*os.File, error) {
		key := split + "/" + name
		if f, ok := files[key]; ok {
			return f, nil
		}
		if err := os.MkdirAll(filepath.Join(out, split), 0o755); err != nil {
			return nil, err
		}
		f, err := os.Create(filepath.Join(out, split, name))
		files[key] = f
		return f, err
	}
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, split := range []string{"train", "eval"} {
		for _, name := range []string{"sft.jsonl", "dpo.jsonl"} {
			if _, err := open(split, name); err != nil {
				return m, err
			}
		}
	}

	// Gather every example across all episode folders first, so an episode
	// found twice, for example as a copy, still counts once.
	examples := map[string]*example{}
	var exampleOrder []string
	versions := map[int]bool{}
	seenEpisodes := map[string]string{}
	for _, ep := range episodes {
		planToExample := map[string]*example{}
		planCandidates := map[string]map[string]engine.RecordCandidate{}
		readLines(filepath.Join(ep.Dir, "plans.jsonl"), func(line []byte) {
			var p engine.PlanRecord
			if json.Unmarshal(line, &p) != nil {
				return
			}
			m.Counts["plan_records"]++
			// Always recomputed, so records from older versions group with
			// newer ones.
			key := engine.ExampleKeyOf(p.Prompt)
			episodeKey := p.Episode.Key
			if episodeKey == "" {
				episodeKey = ep.Name
			}
			// The name is only for reading the manifest. Records carry the
			// file they came from, which says more than the folder now
			// that one folder holds them all.
			if _, ok := seenEpisodes[episodeKey]; !ok {
				name := p.Episode.File
				if name == "" {
					name = ep.Name
				}
				seenEpisodes[episodeKey] = name
			}
			ex, ok := examples[key]
			if !ok {
				ex = &example{key: key, system: p.System, prompt: p.Prompt, episode: episodeKey,
					version: p.PromptVersion, states: map[string]*candidateState{}}
				examples[key] = ex
				exampleOrder = append(exampleOrder, key)
			} else if p.PromptVersion > ex.version {
				ex.system, ex.version = p.System, p.PromptVersion
			}
			versions[p.PromptVersion] = true
			planToExample[p.PlanID] = ex
			planCandidates[p.PlanID] = map[string]engine.RecordCandidate{}
			for _, c := range p.Candidates {
				planCandidates[p.PlanID][c.CID] = c
				ck := candidateKey(c)
				if _, ok := ex.states[ck]; !ok {
					ex.states[ck] = &candidateState{cand: c, order: len(ex.order)}
					ex.order = append(ex.order, ck)
				}
			}
		})
		readLines(filepath.Join(ep.Dir, "decisions.jsonl"), func(line []byte) {
			var d engine.DecisionRecord
			if json.Unmarshal(line, &d) != nil {
				return
			}
			ex := planToExample[d.PlanID]
			cand, ok := planCandidates[d.PlanID][d.CID]
			if ex == nil || !ok {
				return
			}
			m.Counts["decision_records"]++
			st := ex.states[candidateKey(cand)]
			if d.Event == engine.DecisionViewed {
				st.viewed = true
				return
			}
			st.decided = true
			if d.Final != nil {
				st.final = d.Final
				st.changes = d.Changes
				st.changed = d.Changes != nil && !d.Changes.Unchanged
			}
			switch d.Event {
			case engine.DecisionRejected:
				st.rejected = true
				st.reasons = d.Reasons
			case engine.DecisionKept, engine.DecisionEdited:
				st.rejected = false
				if d.Event == engine.DecisionEdited {
					// A render before the edit showed an older version.
					st.rendered = false
					st.edited = true
				}
			case engine.DecisionRendered:
				st.rejected = false
				st.rendered = true
			case engine.DecisionPublished:
				st.rejected = false
				st.published = true
			case engine.DecisionUnpublished:
				st.published = false
			}
		})
	}
	for key, name := range seenEpisodes {
		split := "train"
		if isEval(key, evalShare) {
			split = "eval"
		}
		m.Episodes[split] = append(m.Episodes[split], name)
	}
	for _, split := range []string{"train", "eval"} {
		sort.Strings(m.Episodes[split])
	}

	for _, key := range exampleOrder {
		ex := examples[key]
		split := "train"
		if isEval(ex.episode, evalShare) {
			split = "eval"
		}
		m.Counts["examples"]++
		// Every version so far only extends the one before, so an older
		// record is trained with the current system prompt, which can
		// express everything its targets need.
		system := ex.system
		upgraded := ex.version < engine.PromptVersion
		if upgraded {
			system = engine.SystemPrompt
			m.Counts["upgraded_examples"]++
		}
		prompt := []Message{{"system", system}, {"user", ex.prompt}}

		// Clips that were played but never rendered, edited or kept count as
		// passed over, but only where something else of the same example was
		// rendered or published, so the person was clearly choosing.
		chose := false
		for _, st := range ex.states {
			if !st.rejected && (st.rendered || st.published) {
				chose = true
			}
		}
		passedOver := 0
		var good, bad []answerClip
		var goodMeta []map[string]any
		targets := map[string]bool{}
		weight := 0.0
		var pairs []dpoRecord
		for _, ck := range ex.order {
			st := ex.states[ck]
			proposed := answerClip{Slug: st.cand.Slug, Title: st.cand.Title, Reason: st.cand.Reason,
				Keep: st.cand.Keep}
			if !st.decided {
				if st.viewed && chose {
					bad = append(bad, proposed)
					passedOver++
					m.Counts["passed_over"]++
				}
				continue
			}
			if st.rejected {
				bad = append(bad, proposed)
				m.Counts["rejected"]++
				continue
			}
			target := proposed
			if st.final != nil && len(st.final.Lines) > 0 {
				target.Keep = st.final.Lines
			}
			outcome, w := "kept", weightKept
			switch {
			case st.published:
				outcome, w = "published", weightPublished
			case st.rendered:
				outcome, w = "rendered", weightRendered
			}
			if st.changed {
				m.Counts["changed"]++
			} else if st.rendered || st.published {
				m.Counts["full_hits"]++
			}
			m.Counts[outcome]++
			if tk := fmt.Sprint(target.Keep); !targets[tk] {
				targets[tk] = true
				good = append(good, target)
				goodMeta = append(goodMeta, map[string]any{"cid": st.cand.CID, "outcome": outcome,
					"weight": w, "changed": st.changed, "changes": st.changes})
				weight = math.Max(weight, w)
			}
			// A changed clip teaches its correction directly: the changed
			// lines are better than the proposed ones. Moved edges and cut or
			// restored pauses both change the lines.
			if st.changed && fmt.Sprint(target.Keep) != fmt.Sprint(proposed.Keep) {
				pairs = append(pairs, dpoRecord{Prompt: prompt,
					Chosen:   []Message{answer([]answerClip{target})},
					Rejected: []Message{answer([]answerClip{proposed})},
					Meta:     map[string]any{"kind": "correction", "example": key, "weight": w}})
			}
		}
		if len(good) == 0 {
			continue
		}
		chosen := answer(good)
		if len(bad) > 0 {
			// A selection made only of passed-over clips is a weaker signal
			// than one with an explicit rejection.
			selectionWeight := weight
			if passedOver == len(bad) {
				selectionWeight = math.Min(weight, weightPassedOver)
			}
			pairs = append([]dpoRecord{{Prompt: prompt, Chosen: []Message{chosen},
				Rejected: []Message{answer(bad)},
				Meta: map[string]any{"kind": "selection", "example": key, "weight": selectionWeight,
					"passed_over": passedOver}}}, pairs...)
		}
		f, err := open(split, "sft.jsonl")
		if err == nil {
			err = writeJSON(f, sftRecord{Messages: append(append([]Message{}, prompt...), chosen),
				Meta: map[string]any{"example": key, "weight": weight, "clips": goodMeta,
					"recorded_prompt_version": ex.version, "prompt_version": engine.PromptVersion,
					"system_upgraded": upgraded}})
		}
		if err != nil {
			return m, err
		}
		m.Counts[split+"_sft"]++
		if f, err = open(split, "dpo.jsonl"); err != nil {
			return m, err
		}
		for _, pair := range pairs {
			if err := writeJSON(f, pair); err != nil {
				return m, err
			}
			m.Counts[split+"_dpo"]++
		}
	}
	for v := range versions {
		m.PromptVersion = append(m.PromptVersion, v)
	}
	sort.Ints(m.PromptVersion)
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	return m, os.WriteFile(filepath.Join(out, "manifest.json"), append(body, '\n'), 0o644)
}

func answer(clips []answerClip) Message {
	body, _ := json.Marshal(map[string][]answerClip{"clips": clips})
	return Message{Role: "assistant", Content: string(body)}
}

func writeJSON(f *os.File, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", body)
	return err
}
