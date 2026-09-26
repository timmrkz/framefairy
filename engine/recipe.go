package engine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Recipes
//
// A recipe is one way of asking a model for clips: what it is told, how the
// transcript is written out for it, what shape its answer takes, and how
// that answer is read back. Everything after the answer is the same for
// every recipe. The answer is turned into runs of the transcript's own
// lines, and from there the words, their times, the cuts, the framing and
// the captions are the engine's, to the millisecond, whatever the model
// was shown.
//
// So a recipe may number what it likes, sentences or paragraphs rather than
// lines, as long as each thing it numbers is a run of whole lines. That is
// what lets two ways of asking be run side by side on the same window and
// compared, without either of them touching the timing.
// ---------------------------------------------------------------------------

// Recipe is one way of choosing clips.
type Recipe struct {
	// Name is how it is asked for, with --recipe.
	Name string
	// About says in a line what it tries.
	About string
	// Unit is what it numbers, in a word: line, sentence.
	Unit string
	// Joins is true for a recipe that leaves the pauses to the engine. Runs
	// that follow each other in its answer are one run, and a pause inside
	// a run is cut or kept by the engine's rule, as in any run. Without it,
	// as in lines, two runs that meet say the pause between them is cut.
	Joins bool
	// Edit is true for a recipe that asks a second time, in the same
	// conversation, about where every clip starts and ends. The thinking
	// is split between the two asks, so it thinks no longer in all than a
	// recipe that asks once. See fit.go.
	Edit bool
	// Version is the version of its answer format. See PromptVersion.
	Version int
	// System is what the model is told before the request.
	System string
	// Units groups the lines into the things the model numbers, each a run
	// of whole lines, first and last, counted from zero. Nil numbers every
	// line on its own.
	Units func(lines []Line) [][2]int
	// Request is the request: what is asked for, and the transcript written
	// out with the units numbered from 1.
	Request func(lines []Line, units [][2]int, opts PlanOptions) string
	// Schema is the answer's shape as JSON schema, which a local model is
	// held to while it writes. It has units numbers in it and asks for at
	// most count clips.
	Schema func(units, count int) string
}

// DefaultRecipe is the recipe a search uses unless told otherwise.
const DefaultRecipe = "lines"

// recipes is every recipe there is, by name.
var recipes = map[string]Recipe{
	linesRecipe.Name: linesRecipe,
}

// RecipeNamed gives the recipe of that name, the default one for an empty
// name.
func RecipeNamed(name string) (Recipe, error) {
	if name == "" {
		name = DefaultRecipe
	}
	r, ok := recipes[name]
	if !ok {
		return Recipe{}, renderErr("there is no recipe called %s. There are: %s.",
			pyRepr(name), strings.Join(RecipeNames(), ", "))
	}
	return r, nil
}

// RecipeNames lists the recipes, in order.
func RecipeNames() []string {
	names := make([]string, 0, len(recipes))
	for name := range recipes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsExperiment is true for a recipe other than the default. A search with
// one keeps its plan apart, renders nothing and records nothing for
// training.
func IsExperiment(recipe string) bool { return recipe != "" && recipe != DefaultRecipe }

// PlanFor is where the plan of that name goes: in the logs folder for a
// search, and for an experiment in a folder of its recipe's own, so an
// experiment never takes the place of the plan the app reads. A search
// with any recipe but the default is an experiment, and a comparison makes
// one of the default recipe's too.
func PlanFor(work, recipe string, experiment bool, name string) string {
	if experiment || IsExperiment(recipe) {
		if recipe == "" {
			recipe = DefaultRecipe
		}
		return filepath.Join(work, "experiments", recipe, name)
	}
	return filepath.Join(work, "logs", name)
}

// recipe is the recipe these options ask for. Options that name none, or
// one that does not exist, get the default: a name is checked where it is
// given, see Options.
func (opts PlanOptions) recipe() Recipe {
	r, err := RecipeNamed(opts.Recipe)
	if err != nil {
		r, _ = RecipeNamed("")
	}
	return r
}

// units groups the lines the way the recipe numbers them.
func (r Recipe) units(lines []Line) [][2]int {
	if r.Units != nil {
		return r.Units(lines)
	}
	return lineUnits(len(lines))
}

// lineUnits numbers each of n lines on its own.
func lineUnits(n int) [][2]int {
	units := make([][2]int, max(n, 0))
	for i := range units {
		units[i] = [2]int{i, i}
	}
	return units
}

// toLines turns an answer's runs of units, numbered from 1, into runs of
// lines, numbered from 1, which is what everything after the answer works
// in. Two runs that meet once they are lines stay two runs: the pause
// between them is cut, exactly as the model said.
func toLines(keep [][2]int, units [][2]int) ([][2]int, error) {
	out := make([][2]int, 0, len(keep))
	for _, run := range keep {
		first, last := run[0], run[1]
		if first < 1 || last > len(units) || first > last {
			return nil, fmt.Errorf("units %d-%d are outside 1-%d", first, last, len(units))
		}
		out = append(out, [2]int{units[first-1][0] + 1, units[last-1][1] + 1})
	}
	return out, nil
}

// joinRuns makes one run of runs that follow each other.
func joinRuns(keep [][2]int) [][2]int {
	var out [][2]int
	for _, run := range keep {
		if n := len(out); n > 0 && run[0] == out[n-1][1]+1 {
			out[n-1][1] = run[1]
			continue
		}
		out = append(out, run)
	}
	return out
}

// linesRecipe is how clips have been chosen from the start: every line of
// speech numbered, with its length and any pause and change of level before
// it, and the answer runs of lines. Its prompt and answer are PromptVersion.
var linesRecipe = Recipe{
	Name:    "lines",
	About:   "every line of speech numbered, with its length, pauses and level",
	Unit:    "line",
	Version: PromptVersion,
	System:  SystemPrompt,
	Request: func(lines []Line, _ [][2]int, opts PlanOptions) string { return buildPrompt(lines, opts) },
	Schema:  planSchema,
}
