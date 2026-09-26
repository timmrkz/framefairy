package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Comparing recipes
//
// The same window of the same episode, searched once with each recipe, and
// a report that puts what each cost beside what each found. What each cost
// is measured: the time, the size of the request, and what the local model
// says it read and wrote. What each found is written out as a person reads
// a short, the title and the words that stay, because whether a clip tells
// a story is judged by reading it, not by a number.
//
// Every search in a comparison is an experiment, the default recipe's too,
// so a comparison never takes the place of the episode's own plan. And it
// asks the model afresh each time unless told otherwise: a comparison of
// what things cost is no comparison when one side reuses a saved answer.
// ---------------------------------------------------------------------------

// RecipeRun is one recipe's search of the window.
type RecipeRun struct {
	Recipe  string
	Plan    string
	Seconds float64
	// PromptChars is the whole request, what the model is told and the
	// request with the transcript in it.
	PromptChars int
	// What a local model says it read and wrote, when one was asked.
	PromptTokens, WrittenTokens, ThoughtChars int
	Clips                                     []FoundClip
	Failed                                    string
}

// FoundClip is a clip as the report shows it.
type FoundClip struct {
	Title, Reason string
	Seconds       float64
	Parts         int
	// Text is the words that stay, with a mark where something was cut.
	Text string
}

// Compare searches the window of opts once with each recipe and writes a
// report beside the plans. It gives the runs and the report's path.
func (e *Engine) Compare(ctx context.Context, opts Options, names []string) ([]RecipeRun, string, error) {
	if len(names) == 0 {
		return nil, "", renderErr("name the recipes to compare, for instance --compare lines,stories")
	}
	for _, name := range names {
		if _, err := RecipeNamed(name); err != nil {
			return nil, "", err
		}
	}
	work := WorkDir(opts.Source)
	logs := filepath.Join(work, "logs")
	var runs []RecipeRun
	for _, name := range names {
		if ctx.Err() != nil {
			return runs, "", ctx.Err()
		}
		e.Log.Info("searching with the %s recipe", name)
		o := opts
		o.Recipe, o.Experiment, o.PlanOnly = name, true, true
		began := time.Now()
		// The hook can be called from the goroutines that frame the clips.
		var mu sync.Mutex
		failed := ""
		e.Log.SetErrorHook(func(text string) {
			mu.Lock()
			failed = text
			mu.Unlock()
		})
		code := e.Run(ctx, o)
		e.Log.SetErrorHook(nil)
		run := RecipeRun{Recipe: name, Seconds: time.Since(began).Seconds()}
		if code != 0 {
			mu.Lock()
			run.Failed = failed
			mu.Unlock()
			if run.Failed == "" {
				run.Failed = "the search failed"
			}
		}
		if body, err := os.ReadFile(filepath.Join(logs, "plan-prompt.txt")); err == nil {
			run.PromptChars = runeLen(string(body))
		}
		run.PromptTokens, run.WrittenTokens, run.ThoughtChars = lastLocalAnswer(logs, began)
		run.Plan = newestPlan(filepath.Join(work, "experiments", name), began)
		if run.Plan != "" {
			run.Clips = foundClips(run.Plan)
		}
		runs = append(runs, run)
	}
	// A report of nothing but failures says nothing a log line did not, and
	// a line that says where it is reads as if the comparison worked.
	if every(runs, func(r RecipeRun) bool { return r.Failed != "" }) {
		return runs, "", renderErr("every search failed, so there is nothing to compare: %s",
			runs[len(runs)-1].Failed)
	}
	report := filepath.Join(work, "experiments",
		"compare-"+time.Now().Format("2006-01-02-150405")+".md")
	if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil {
		return runs, "", err
	}
	if err := os.WriteFile(report, []byte(compareReport(opts, runs)), 0o644); err != nil {
		return runs, "", err
	}
	return runs, report, nil
}

// lastLocalAnswer reads what the local model said about the answer saved
// since began: the tokens it read, the tokens it wrote, and how much of what
// it wrote was thought. All zero for the API, or an answer reused.
func lastLocalAnswer(logs string, began time.Time) (read, written, thought int) {
	matches, _ := filepath.Glob(filepath.Join(logs, "reply-*.json"))
	var newest string
	var at time.Time
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.ModTime().Before(began) || info.ModTime().Before(at) {
			continue
		}
		newest, at = m, info.ModTime()
	}
	if newest == "" {
		return 0, 0, 0
	}
	body, err := os.ReadFile(newest)
	if err != nil {
		return 0, 0, 0
	}
	var saved struct {
		How *localAnswer `json:"how"`
	}
	if json.Unmarshal(body, &saved) != nil || saved.How == nil {
		return 0, 0, 0
	}
	return saved.How.PromptTokens, saved.How.Written, saved.How.Reasoning
}

// newestPlan is the plan written in dir since began.
func newestPlan(dir string, began time.Time) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "clips*.json"))
	sort.Slice(matches, func(a, b int) bool {
		ia, _ := os.Stat(matches[a])
		ib, _ := os.Stat(matches[b])
		return ia != nil && ib != nil && ia.ModTime().After(ib.ModTime())
	})
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && !info.ModTime().Before(began) {
			return m
		}
	}
	return ""
}

// foundClips reads a plan the way the engine does, and writes each clip
// out as it is heard.
func foundClips(path string) []FoundClip {
	plan, clips, err := LoadClips(path)
	if err != nil {
		return nil
	}
	reasons := map[string]string{}
	if list, ok := plan.Raw["clips"].([]any); ok {
		for _, item := range list {
			if c, ok := item.(map[string]any); ok {
				id, _ := c["id"].(string)
				reasons[id], _ = c["reason"].(string)
			}
		}
	}
	var out []FoundClip
	for _, c := range clips {
		found := FoundClip{Title: c.Title, Reason: reasons[c.ID], Parts: len(c.Segments)}
		var text strings.Builder
		for i, seg := range c.Segments {
			found.Seconds += seg.End - seg.Start
			if i > 0 {
				text.WriteString(" [...]")
			}
			for _, w := range c.Words {
				if w.Start >= seg.Start-0.001 && w.Start < seg.End {
					text.WriteString(" " + w.Text)
				}
			}
		}
		found.Text = strings.TrimSpace(text.String())
		out = append(out, found)
	}
	return out
}

func compareReport(opts Options, runs []RecipeRun) string {
	var b strings.Builder
	window := "the whole episode"
	if opts.From != "" || opts.To != "" {
		window = fmt.Sprintf("from %s to %s", orDash(opts.From), orDash(opts.To))
	}
	fmt.Fprintf(&b, "# Recipes compared\n\n%s, %s, %d clips of %s to %s seconds asked for.\n\n",
		filepath.Base(opts.Source), window, opts.Count, fixed(opts.Min, 0), fixed(opts.Max, 0))
	b.WriteString("| Recipe | Clips | Seconds | Request, characters | Read, tokens | Written, tokens | Thought, characters |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range runs {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s |\n", r.Recipe, len(r.Clips),
			fixed(r.Seconds, 0), commas(r.PromptChars), orNone(r.PromptTokens),
			orNone(r.WrittenTokens), orNone(r.ThoughtChars))
	}
	for _, r := range runs {
		fmt.Fprintf(&b, "\n## %s\n\n", r.Recipe)
		if recipe, err := RecipeNamed(r.Recipe); err == nil {
			b.WriteString(recipe.About + ".\n\n")
		}
		if r.Failed != "" {
			fmt.Fprintf(&b, "The search failed: %s\n", r.Failed)
			continue
		}
		for i, c := range r.Clips {
			fmt.Fprintf(&b, "### %d. %s\n\n%s seconds, %d part(s). %s\n\n> %s\n\n",
				i+1, c.Title, fixed(c.Seconds, 1), c.Parts, c.Reason, c.Text)
		}
	}
	return b.String()
}

func every(runs []RecipeRun, test func(RecipeRun) bool) bool {
	for _, r := range runs {
		if !test(r) {
			return false
		}
	}
	return true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orNone(n int) string {
	if n == 0 {
		return "-"
	}
	return commas(n)
}
