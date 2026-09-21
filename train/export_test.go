package train

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/engine"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<24)
	for s.Scan() {
		var v map[string]any
		if err := json.Unmarshal(s.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

const plan1 = `{"plan_id":"p1","example_key":"ex1","prompt_version":1,"system":"S","prompt":"P",` +
	`"episode":{"file":"ep1.mp4","key":"k1"},"candidates":[` +
	`{"cid":"01","slug":"a","title":"A","reason":"r","keep":[[1,3]],"segments":[[0,9]]},` +
	`{"cid":"02","slug":"b","title":"B","reason":"r","keep":[[5,6]],"segments":[[20,25]]},` +
	`{"cid":"03","slug":"c","title":"C","reason":"r","keep":[[8,9]],"segments":[[30,35]]},` +
	`{"cid":"04","slug":"d","title":"D","reason":"r","keep":[[11,12]],"segments":[[40,45]]},` +
	`{"cid":"05","slug":"f","title":"F","reason":"r","keep":[[16,19]],"segments":[[60,70]]}]}`

// The same prompt answered again after the episode was reset, with one clip
// the same as before and one new.
const plan2 = `{"plan_id":"p2","example_key":"ex1","prompt_version":1,"system":"S","prompt":"P",` +
	`"episode":{"file":"ep1.mp4","key":"k1"},"candidates":[` +
	`{"cid":"01","slug":"a","title":"A","reason":"r","keep":[[1,3]],"segments":[[0,9]]},` +
	`{"cid":"02","slug":"e","title":"E","reason":"r","keep":[[14,15]],"segments":[[50,55]]}]}`

func TestExportCountsEachSignalOnce(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ep1.framefairy", "training")
	writeFile(t, filepath.Join(dir, "plans.jsonl"), plan1+"\n"+plan2+"\nnot json\n")
	writeFile(t, filepath.Join(dir, "decisions.jsonl"), strings.Join([]string{
		// 01 rendered as proposed, twice, once in each plan: one full hit.
		`{"plan_id":"p1","cid":"01","event":"rendered","final":{"lines":[[1,3]],"segments":[[0,9]]},"changes":{"unchanged":true}}`,
		`{"plan_id":"p2","cid":"01","event":"rendered","final":{"lines":[[1,3]],"segments":[[0,9]]},"changes":{"unchanged":true}}`,
		// 02 kept, then rejected with a reason: rejected wins.
		`{"plan_id":"p1","cid":"02","event":"kept","final":{"lines":[[5,6]],"segments":[[20,25]]}}`,
		`{"plan_id":"p1","cid":"02","event":"rejected","reasons":["boring"]}`,
		// 03 trimmed and rendered: a correction.
		`{"plan_id":"p1","cid":"03","event":"edited","final":{"lines":[[8,10]],"segments":[[30,38]]},"changes":{"unchanged":false,"end_shift":3}}`,
		`{"plan_id":"p1","cid":"03","event":"rendered","final":{"lines":[[8,10]],"segments":[[30,38]]},"changes":{"unchanged":false,"end_shift":3}}`,
		// 05 had a pause cut between lines 17 and 18, then was rendered.
		`{"plan_id":"p1","cid":"05","event":"rendered","final":{"lines":[[16,17],[18,19]],"segments":[[60,64],[65,70]]},"changes":{"unchanged":false,"pauses_cut":[[64,65]]}}`,
		// 04 only played: passed over, since other clips were rendered.
		`{"plan_id":"p1","cid":"04","event":"viewed"}`,
		// Nothing else about 04. The new clip of plan 2 is published.
		`{"plan_id":"p2","cid":"02","event":"published","final":{"lines":[[14,15]],"segments":[[50,55]]},"changes":{"unchanged":true}}`,
	}, "\n")+"\n")
	// A copy of the same episode under another name counts once.
	copyDir := filepath.Join(root, "copy.framefairy", "training")
	for _, name := range []string{"plans.jsonl", "decisions.jsonl"} {
		body, _ := os.ReadFile(filepath.Join(dir, name))
		writeFile(t, filepath.Join(copyDir, name), string(body))
	}

	episodes, err := FindEpisodes(root)
	if err != nil || len(episodes) != 2 {
		t.Fatalf("episodes %v %v", episodes, err)
	}
	out := filepath.Join(root, "dataset")
	m, err := Export(episodes, out, 0)
	if err != nil {
		t.Fatal(err)
	}
	sft := readJSONL(t, filepath.Join(out, "train", "sft.jsonl"))
	dpo := readJSONL(t, filepath.Join(out, "train", "dpo.jsonl"))
	if len(sft) != 1 || len(dpo) != 3 {
		t.Fatalf("%d sft and %d dpo records, want 1 and 3", len(sft), len(dpo))
	}
	answer := sft[0]["messages"].([]any)[2].(map[string]any)["content"].(string)

	t.Run("counts", func(t *testing.T) {
		want := map[string]int{"examples": 1, "full_hits": 2, "changed": 2, "rejected": 1,
			"passed_over": 1, "rendered": 3, "published": 1, "train_sft": 1, "train_dpo": 3,
			"upgraded_examples": 1}
		for k, v := range want {
			if m.Counts[k] != v {
				t.Errorf("%s = %d, want %d (all %v)", k, m.Counts[k], v, m.Counts)
			}
		}
		if len(m.Episodes["train"]) != 1 {
			t.Errorf("the same episode under two names counted as %v", m.Episodes)
		}
	})

	t.Run("the target holds the clips that were used", func(t *testing.T) {
		for _, want := range []string{`"keep":[[1,3]]`, `"keep":[[8,10]]`, `"keep":[[14,15]]`,
			`"keep":[[16,17],[18,19]]`} {
			if !strings.Contains(answer, want) {
				t.Errorf("target misses %s: %s", want, answer)
			}
		}
		if strings.Contains(answer, `"slug":"b"`) || strings.Contains(answer, `"slug":"d"`) ||
			strings.Count(answer, `"slug":"a"`) != 1 {
			t.Errorf("target has rejected, undecided or doubled clips: %s", answer)
		}
		if sft[0]["meta"].(map[string]any)["weight"].(float64) != weightPublished {
			t.Errorf("weight %v", sft[0]["meta"])
		}
	})

	t.Run("the pairs", func(t *testing.T) {
		kinds := []string{}
		for _, d := range dpo {
			kinds = append(kinds, d["meta"].(map[string]any)["kind"].(string))
		}
		if strings.Join(kinds, ",") != "selection,correction,correction" {
			t.Errorf("pairs %v", kinds)
		}
		selection := dpo[0]["rejected"].([]any)[0].(map[string]any)["content"].(string)
		if !strings.Contains(selection, `"slug":"b"`) || !strings.Contains(selection, `"slug":"d"`) {
			t.Errorf("selection pair should hold the rejected and the passed-over clip: %s", selection)
		}
		correction := dpo[1]
		chosen := correction["chosen"].([]any)[0].(map[string]any)["content"].(string)
		rejected := correction["rejected"].([]any)[0].(map[string]any)["content"].(string)
		if !strings.Contains(chosen, `[[8,10]]`) || !strings.Contains(rejected, `[[8,9]]`) {
			t.Errorf("correction pair %s over %s", chosen, rejected)
		}
	})

	t.Run("an older record is trained with the current prompt", func(t *testing.T) {
		system := sft[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
		if system != engine.SystemPrompt || sft[0]["meta"].(map[string]any)["system_upgraded"] != true {
			t.Errorf("a version 1 record was not given the current system prompt")
		}
	})

	t.Run("the split", func(t *testing.T) {
		if len(readJSONL(t, filepath.Join(out, "eval", "sft.jsonl"))) != 0 {
			t.Errorf("eval should be empty with share 0")
		}
		if !isEval("k1", 1.0) || isEval("k1", 0) {
			t.Errorf("split bounds")
		}
	})
}

// TestTheSplitIsTheSameEverywhere pins the held-out split to the episode
// name alone. It has to come out the same on every machine and in every
// export, or an episode would move between training and evaluation and the
// numbers would stop meaning anything.
func TestTheSplitIsTheSameEverywhere(t *testing.T) {
	cases := []struct {
		name  string
		where float64
	}{
		{"k1", 0.4169},
		{"ep1", 0.9586},
		{"copy", 0.4350},
		{"My First Memory 12", 0.6703},
	}
	for _, c := range cases {
		if !isEval(c.name, c.where+0.0001) {
			t.Errorf("%s should be held out below %.4f", c.name, c.where)
		}
		if isEval(c.name, c.where-0.0001) {
			t.Errorf("%s should be trained on above %.4f", c.name, c.where)
		}
	}
}

// TestFindEpisodesTakesOnlyTrainingFolders keeps the export from walking
// into whatever else lives next to an episode.
func TestFindEpisodesTakesOnlyTrainingFolders(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.framefairy", "training", "plans.jsonl"), plan1+"\n")
	writeFile(t, filepath.Join(root, "a.framefairy", "training", "plans.jsonl"), plan1+"\n")
	// No plans, not a .framefairy folder, and a training folder nested deeper.
	writeFile(t, filepath.Join(root, "c.framefairy", "training", "decisions.jsonl"), "")
	writeFile(t, filepath.Join(root, "notes", "training", "plans.jsonl"), plan1+"\n")
	writeFile(t, filepath.Join(root, "d.framefairy", "logs", "clips.json"), "{}")

	episodes, err := FindEpisodes(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ep := range episodes {
		names = append(names, ep.Name)
	}
	if strings.Join(names, ",") != "a,b" {
		t.Errorf("found %v", names)
	}
}
