package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	// Use is what a local model says it read, thought and wrote, and how
	// long that took, when one was asked.
	Use    modelUse
	Clips  []FoundClip
	Failed string
}

// FoundClip is a clip as the report shows it.
type FoundClip struct {
	Title, Reason string
	Seconds       float64
	// Start and End are where in the episode the clip begins and ends.
	Start, End float64
	Parts      int
	// Text is the words that stay, with a mark where something was cut.
	Text string
}

// Variant is one side of a comparison: a recipe, and how long the model
// may think with it when that is given, written stories@1024. Two sides
// may be the same recipe thinking for longer and shorter.
type Variant struct {
	Recipe string
	// Think is the thinking budget in tokens, nil for the search's own.
	Think *int
	// Temperature is how freely the model picks its words, nil for the
	// search's own.
	Temperature *float64
	// Switches are what the side shows the model beyond the recipe's own,
	// written after a +, as in points+times.
	Switches PromptSwitches
}

// ParseVariant reads a side of a comparison: a recipe name, after an @ the
// tokens it may think, -1 for no limit, and after a ~ its temperature, as
// in stories@1024~0.3.
func ParseVariant(name string) (Variant, error) {
	rest, temperature, hasTemperature := strings.Cut(strings.TrimSpace(name), "~")
	var t *float64
	if hasTemperature {
		f, err := strconv.ParseFloat(temperature, 64)
		if err != nil || f < 0 || f > 2 || math.IsNaN(f) {
			return Variant{}, renderErr("%s: after the ~ goes the temperature, from 0 to 2, "+
				"for instance stories~0.3", pyRepr(name))
		}
		t = &f
	}
	recipe, think, hasThink := strings.Cut(rest, "@")
	recipe, switches, hasSwitches := strings.Cut(recipe, "+")
	r, err := RecipeNamed(recipe)
	if err != nil || recipe == "" {
		if err == nil {
			err = renderErr("a side of a comparison needs a recipe, as in stories@1024")
		}
		return Variant{}, err
	}
	v := Variant{Recipe: recipe, Temperature: t}
	if hasSwitches {
		if !r.Switchable {
			return Variant{}, renderErr("%s: %s takes no switches. heart-lean, points and middle do, "+
				"as in middle+times", pyRepr(name), recipe)
		}
		sw, err := ParseSwitches(switches)
		if err != nil {
			return Variant{}, renderErr("%s: %s", pyRepr(name), err)
		}
		v.Switches = sw
	}
	if hasThink {
		n, err := strconv.Atoi(think)
		if err != nil || n < -1 {
			return Variant{}, renderErr("%s: after the @ goes how many tokens the model may think, "+
				"for instance stories@1024, or -1 for no limit", pyRepr(name))
		}
		v.Think = &n
	}
	return v, nil
}

// compareSeed is the seed of a comparison that is given none.
const compareSeed = 1

// Compare searches the window of opts once with each recipe and writes a
// report beside the plans. It gives the runs and the report's path.
func (e *Engine) Compare(ctx context.Context, opts Options, names []string) ([]RecipeRun, string, error) {
	if len(names) == 0 {
		return nil, "", renderErr("name the recipes to compare, for instance --compare lines,stories")
	}
	variants := make([]Variant, len(names))
	for i, name := range names {
		v, err := ParseVariant(name)
		if err != nil {
			return nil, "", err
		}
		variants[i] = v
	}
	work := WorkDir(opts.Source)
	logs := filepath.Join(work, "logs")
	var runs []RecipeRun
	for i, name := range names {
		if ctx.Err() != nil {
			return runs, "", ctx.Err()
		}
		// Every side starts with a llama-server of its own, so none finds
		// the request of the side before in its cache. Two sides that
		// differ only in thinking send the same request, and the second
		// read 15,303 tokens in 0.1 seconds where the first took 26. A
		// server somebody started themselves is theirs to restart.
		if i > 0 && opts.LLMURL == "" {
			StopModels()
		}
		e.Log.Info("searching with the %s recipe", name)
		o := opts
		o.Recipe, o.Variant, o.Experiment, o.PlanOnly = variants[i].Recipe, name, true, true
		if variants[i].Think != nil {
			o.Think = *variants[i].Think
		}
		if variants[i].Temperature != nil {
			o.Temperature = variants[i].Temperature
		}
		o.Switches = variants[i].Switches
		// Every side draws the same way, so what differs between two is
		// what was changed, not the luck of the draw.
		if o.Seed == 0 {
			o.Seed = compareSeed
		}
		began := time.Now()
		err := e.execute(ctx, o)
		run := RecipeRun{Recipe: name, Seconds: time.Since(began).Seconds()}
		if err != nil {
			if !errors.Is(err, ErrCancelled) {
				e.Log.Error("%s", err)
			}
			run.Failed = err.Error()
		}
		// What was asked and what came back are kept beside the plan, where
		// the next recipe's search does not write over them.
		dir := filepath.Join(work, "experiments", name)
		if body, err := os.ReadFile(filepath.Join(logs, "plan-prompt.txt")); err == nil &&
			!modifiedBefore(filepath.Join(logs, "plan-prompt.txt"), began) {
			run.PromptChars = runeLen(string(body))
			keepCopy(dir, "prompt.txt", body)
		}
		var reply string
		reply, run.Use = lastLocalAnswer(logs, began)
		if body, err := os.ReadFile(reply); reply != "" && err == nil {
			keepCopy(dir, "reply.json", body)
		}
		run.Plan = newestPlan(dir, began)
		if run.Plan != "" {
			heard, err := SavedTranscript(opts.Source, logs, opts.ASRModel, opts.SilenceDB)
			if err != nil {
				heard = &Transcript{}
			}
			run.Clips = foundClips(run.Plan, heard.Words)
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

// modifiedBefore is true for a file last written before t, or not there.
func modifiedBefore(path string, t time.Time) bool {
	info, err := os.Stat(path)
	return err != nil || info.ModTime().Before(t)
}

// keepCopy writes a copy into dir. A copy that cannot be written costs the
// report nothing, so it is only left out.
func keepCopy(dir, name string, body []byte) {
	if os.MkdirAll(dir, 0o755) == nil {
		_ = os.WriteFile(filepath.Join(dir, name), body, 0o644)
	}
}

// modelUse is what a local model did for a search. Asks is how many times
// it was asked, the first ask and every ask again about clips well off the
// length. Read is the tokens of the first request, and ReadAnew those it
// read rather than found in llama-server's cache from the side before,
// which has the same request when two sides differ only in thinking.
// Reading is the seconds that reading took. Thought is the tokens of
// thought, or where they were not counted, ThoughtChars its characters.
// Written and Seconds are of every ask.
type modelUse struct {
	Asks, Read, ReadAnew, Thought, ThoughtChars, Written int
	Reading, Seconds                                     float64
}

// lastLocalAnswer finds the answer saved since began and reads what the
// local model said about it. It is all zero for the API, and the path empty
// for an answer reused.
func lastLocalAnswer(logs string, began time.Time) (path string, use modelUse) {
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
		return "", use
	}
	body, err := os.ReadFile(newest)
	if err != nil {
		return "", use
	}
	var saved struct {
		How      *localAnswer `json:"how"`
		Fit      *string      `json:"fit"`
		FitHow   *localAnswer `json:"fit_how"`
		Refit    *string      `json:"refit"`
		RefitHow *localAnswer `json:"refit_how"`
	}
	if json.Unmarshal(body, &saved) != nil || saved.How == nil {
		return newest, use
	}
	first := saved.How
	use = modelUse{Asks: 1, Read: first.PromptTokens, ReadAnew: first.PromptTokens,
		Thought: first.ReasoningTokens, ThoughtChars: first.Reasoning}
	if first.Timings != nil {
		use.ReadAnew, use.Reading = first.Timings.PromptN, first.Timings.PromptMS/1000
	}
	for _, h := range []*localAnswer{saved.How, saved.FitHow, saved.RefitHow} {
		if h == nil {
			continue
		}
		use.Written += h.Written
		if h.Timings != nil {
			use.Seconds += (h.Timings.PromptMS + h.Timings.PredictedMS) / 1000
		}
	}
	for _, again := range []*string{saved.Fit, saved.Refit} {
		if again != nil {
			use.Asks++
		}
	}
	return newest, use
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
// out as it is heard, in the episode's words.
func foundClips(path string, words []Cue) []FoundClip {
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
		if n := len(c.Segments); n > 0 {
			found.Start, found.End = c.Segments[0].Start, c.Segments[n-1].End
		}
		var text strings.Builder
		for i, seg := range c.Segments {
			found.Seconds += seg.End - seg.Start
			if i > 0 {
				text.WriteString(" [...]")
			}
			for _, w := range Said(Clip{Segments: []Segment{seg}}, words) {
				text.WriteString(" " + w.Text)
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
	seed := opts.Seed
	if seed == 0 {
		seed = compareSeed
	}
	temperature := "llama-server's own, 0.8"
	if opts.Temperature != nil {
		temperature = trimFloat(*opts.Temperature)
	}
	// No count is the count the window suggests, which was never 0 clips.
	count := "as many clips as the window suggests"
	if opts.Count > 0 {
		count = fmt.Sprintf("%d clips", opts.Count)
	}
	fmt.Fprintf(&b, "# Recipes compared\n\n%s, %s, %s of %s to %s seconds asked for. "+
		"Seed %d, temperature %s unless a side says otherwise.\n\n",
		filepath.Base(opts.Source), window, count, fixed(opts.Min, 0), fixed(opts.Max, 0),
		seed, temperature)
	costs := [][]string{{"Recipe", "Clips", "Asks", "Seconds", "Model, seconds", "Reading, seconds",
		"Request, characters", "Read, tokens", "Read anew, tokens", "Thought, tokens", "Written, tokens"}}
	for _, r := range runs {
		u := r.Use
		seconds := func(s float64) string {
			if s <= 0 {
				return "-"
			}
			return fixed(s, 0)
		}
		thought := orNone(u.Thought)
		if u.Thought == 0 && u.ThoughtChars > 0 {
			thought = "about " + commas(u.ThoughtChars*10/25)
		}
		costs = append(costs, []string{r.Recipe, strconv.Itoa(len(r.Clips)), orNone(u.Asks),
			fixed(r.Seconds, 0), seconds(u.Seconds), seconds(u.Reading), commas(r.PromptChars),
			orNone(u.Read), orNone(u.ReadAnew), thought, orNone(u.Written)})
	}
	b.WriteString(alignedTable(costs))
	b.WriteString("\nAsks counts the first ask and every ask again about clips well off the " +
		"length. Model, seconds and Written, tokens are of all of them, the reading of the " +
		"first. Read anew is what the model read rather than found in its cache: a side with " +
		"the same request as the side before reads almost nothing, so its seconds leave out " +
		"the reading the others paid for. Thought is part of Written. Where the tokens of " +
		"thought were not counted, it is worked out from the characters, about 2.5 a token.\n")
	// What can be counted about the clips, beside what has to be read.
	b.WriteString("\nWhat can be counted, fewer is better. A clip starts mid-sentence when its first " +
		"word is in lower case, ends mid-sentence when its last has no full stop, question or " +
		"exclamation mark, and is off the length when it runs under 90 % of the shortest or over " +
		"120 % of the longest asked for.\n\n")
	faults := [][]string{{"Recipe", "Starts mid-sentence", "Ends mid-sentence", "Off the length"}}
	for _, r := range runs {
		starts, ends, off := clipFaults(r.Clips, opts.Min, opts.Max)
		faults = append(faults, []string{r.Recipe, strconv.Itoa(starts), strconv.Itoa(ends),
			strconv.Itoa(off)})
	}
	b.WriteString(alignedTable(faults))
	b.WriteString(momentsTable(runs))
	for _, r := range runs {
		fmt.Fprintf(&b, "\n## %s\n\n", r.Recipe)
		if v, err := ParseVariant(r.Recipe); err == nil {
			recipe, _ := RecipeNamed(v.Recipe)
			b.WriteString(recipe.About)
			if v.Think != nil {
				fmt.Fprintf(&b, ", thinking at most %s tokens", commas(*v.Think))
			}
			if v.Temperature != nil {
				fmt.Fprintf(&b, ", at a temperature of %s", trimFloat(*v.Temperature))
			}
			if sw := v.Switches; sw != (PromptSwitches{}) {
				var shown []string
				if sw.Pause > 0 {
					shown = append(shown, "a mark before a pause of "+trimFloat(sw.Pause)+" s or more")
				}
				if sw.Times {
					shown = append(shown, "the time each line starts at")
				}
				b.WriteString(", with " + strings.Join(shown, ", "))
			}
			b.WriteString(".\n\n")
		}
		if r.Failed != "" {
			fmt.Fprintf(&b, "The search failed: %s\n", r.Failed)
			continue
		}
		for i, c := range r.Clips {
			fmt.Fprintf(&b, "### %d. %s\n\nAt %s, %s seconds, %d part(s). %s\n\n> %s\n\n",
				i+1, c.Title, HMS(c.Start), fixed(c.Seconds, 1), c.Parts, c.Reason, c.Text)
		}
	}
	return b.String()
}

// momentsTable is every moment the sides found, one row each in the order
// they come in the episode, and for each side the seconds of its clip of
// that moment. Two clips are one moment when they share more than half of
// the shorter. So the same story can be read side by side, and a story only
// one side found stands out.
func momentsTable(runs []RecipeRun) string {
	type moment struct {
		start, end float64
		title      string
		seconds    map[int]float64
	}
	var moments []*moment
	for side, r := range runs {
		for _, c := range r.Clips {
			var found *moment
			for _, m := range moments {
				shared := min(m.end, c.End) - max(m.start, c.Start)
				if shared > 0.5*min(m.end-m.start, c.End-c.Start) {
					found = m
					break
				}
			}
			if found == nil {
				found = &moment{start: c.Start, end: c.End, title: c.Title, seconds: map[int]float64{}}
				moments = append(moments, found)
			}
			if _, has := found.seconds[side]; !has {
				found.seconds[side] = c.Seconds
			}
		}
	}
	if len(moments) == 0 {
		return ""
	}
	sort.SliceStable(moments, func(i, j int) bool { return moments[i].start < moments[j].start })
	rows := [][]string{{"Moment"}}
	for _, r := range runs {
		rows[0] = append(rows[0], r.Recipe)
	}
	for _, m := range moments {
		title := []rune(m.title)
		if len(title) > 40 {
			title = append(title[:39], '…')
		}
		row := []string{HMS(m.start) + " " + string(title)}
		for side := range runs {
			cell := "-"
			if s, ok := m.seconds[side]; ok {
				cell = fixed(s, 0) + " s"
			}
			row = append(row, cell)
		}
		rows = append(rows, row)
	}
	return "\nThe moments found, in the order they come, and how long each side's clip of " +
		"it runs. Two clips are one moment when they share more than half of the shorter.\n\n" +
		alignedTable(rows)
}

// alignedTable is a markdown table whose columns line up in a terminal
// too: every cell padded to its column's width, the first column to the
// left and the numbers to the right. The first row is the heads.
func alignedTable(rows [][]string) string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], runeLen(cell), 3)
		}
	}
	var b strings.Builder
	line := func(row []string) {
		b.WriteString("|")
		for i, cell := range row {
			pad := strings.Repeat(" ", widths[i]-runeLen(cell))
			if i == 0 {
				b.WriteString(" " + cell + pad + " |")
			} else {
				b.WriteString(" " + pad + cell + " |")
			}
		}
		b.WriteString("\n")
	}
	line(rows[0])
	b.WriteString("|")
	for i, w := range widths {
		if i == 0 {
			b.WriteString(" " + strings.Repeat("-", w) + " |")
		} else {
			b.WriteString(" " + strings.Repeat("-", w-1) + ": |")
		}
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		line(row)
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

// clipFaults counts the clips that start or end mid-sentence and the ones
// well off the length, the same bounds the log flags.
func clipFaults(clips []FoundClip, least, most float64) (starts, ends, off int) {
	for _, c := range clips {
		text := strings.TrimSpace(c.Text)
		if startsLower(text) {
			starts++
		}
		if !endsSentence(text) {
			ends++
		}
		if c.Seconds < least*0.9 || c.Seconds > most*1.2 {
			off++
		}
	}
	return starts, ends, off
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
