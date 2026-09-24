package engine

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// The language models somebody can install, and installing one.
//
// No language model ships with the app. The smallest useful one is gigabytes
// and the good ones are more, which is not an installer, so finding clips is
// a choice made once: an Anthropic API key, which works on any machine and
// costs per episode, or one of these, which is free per run and needs the
// machine for it. See docs/PACKAGING.md.
//
// A model runs from memory, so what decides whether a machine can have one
// is not the download but what it takes to run. That is the whole of the
// capacity part below: the list is filtered by what this machine holds
// rather than offering somebody a model that will swap for minutes an
// answer.

// LanguageModel is one model that can find clips.
type LanguageModel struct {
	// Name is the file it lands as, in the models folder.
	Name string `json:"name"`
	// Title is what it is called in the app, Maker who made it. They
	// are apart because a list of models is read by maker first: Google's
	// Gemma and Meta's Llama are the same kind of thing from two houses.
	Title string `json:"title"`
	Maker string `json:"maker"`
	// About is one line saying what it is for.
	About string `json:"about"`
	// Download is how big the file is, in bytes, read off the real file
	// rather than guessed. It is said before anything starts, because a
	// download nobody agreed to is a download nobody wanted.
	Download int64 `json:"download"`
	// Needs is the memory it takes to run a search, in bytes: the weights,
	// the cache the context lives in, the checkpoints of the window, and
	// llama.cpp's own working memory.
	// Worked out by NeedsAt at judgedContext, never written by hand.
	Needs int64 `json:"needs"`
	// Context is the most tokens the model holds at once, read off the
	// real file, where llama.cpp reads it too: the context_length in the
	// GGUF header. It is what makes the longest window a search may have,
	// see room.go.
	Context int `json:"-"`
	// Shape is what the model keeps in memory for every token of context.
	// It decides most of the difference between models, see Shape.
	Shape Shape `json:"-"`
	// URL is where it comes from, whoever published it rather than us. Nothing
	// is redistributed, so its licence is between the user and whoever
	// published it.
	URL string `json:"url"`
	// SHA256 of the file, where it is pinned. Empty means it is not, and
	// then the check is that what arrived is a model at all rather than
	// that it is exactly the one expected.
	SHA256 string `json:"-"`
}

// What a model needs in memory is its weights, plus the cache its context
// lives in, plus llama.cpp's working memory. The first is the file. The
// third is small, and so are the checkpoints below. The second is what differs, and by more than anything
// else about these models.
//
// It used to be a quarter of the file, taken from the eighteen gigabytes
// always quoted for Gemma 4 26B. That fit Gemma by the luck of its design
// and nothing else, and applied to the models added after it, it was wrong
// by more than half: it told a 16 GB Mac to install Ministral 3 8B and a
// 24 GB Mac to install Qwen3 14B, each as the best for that machine, and
// neither fits the machine it was offered to.
//
// The reason is how each model attends. Every layer that looks back over
// the whole context keeps a key and a value for every token in it, so its
// cache grows with the length of the search. Gemma 4 lets most of its
// layers look back over only the last 1024 tokens, so a longer search
// costs it almost nothing. Qwen3 and Ministral have no such layers, and
// every token of transcript costs them in every layer. Same file size,
// very different memory.

// Shape is what a model keeps in memory per token of context, read off
// the model's own config.json on Hugging Face rather than remembered.
type Shape struct {
	// Layers that look back over the whole context, and how many key and
	// value heads each has, how wide.
	FullLayers, FullKVHeads, FullHeadDim int
	// Layers that look back over only the last Window tokens.
	WindowLayers, WindowKVHeads, WindowHeadDim, Window int
}

// judgedContext is the context a model is judged at when a machine is
// asked whether it can hold it. 65536 tokens is what contextFor gives a
// search of up to about an hour and a half of episode, which covers the
// first search, the first half hour taken whole up to 45 minutes, with a
// lot to spare. A search longer than that asks for more.
const judgedContext = 65536

// windowSpare is how many cells llama.cpp keeps beyond the window in a
// sliding window cache: one batch, 512 by default. With one slot the cache
// is the window plus this. See llama-kv-cache-iswa.cpp in llama.cpp.
const windowSpare = 512

// workingAllowance is llama.cpp's own working memory on top of the weights
// and the cache: the buffers a batch is computed in, and the output.
// Measured on an M2 Max with Gemma 4 26B A4B at 65536 tokens, it is 415 MiB
// on the graphics side, 77 to 153 MiB on the processor and 1 MiB of
// output, a little over half a gigabyte. The other models have not been
// measured, and a wider model or a larger vocabulary takes more, so the
// allowance is a whole gigabyte.
const workingAllowance = 1 << 30

// checkpoints is how many copies of the window llama-server keeps while it
// reads a prompt, in ordinary memory, so a following ask that shares the
// start of the prompt does not read it all again. It makes one where a
// user message starts and two near the end of the prompt, and a search
// sends one user message, so it is three however long the episode. The
// measured three were 144, 200 and 200 MiB for Gemma 4 26B A4B. A model
// with no window keeps none, because its cache can be cut back instead.
const checkpoints = 3

func perToken(layers, heads, dim int) int64 { return int64(2 * layers * heads * dim * 2) }

// cacheBytes is the key and value cache for a context of ctx tokens, at
// two bytes a number, which is llama.cpp's default. Every layer keeps a key
// and a value for each of its heads. For Gemma 4 26B A4B at 65536 tokens
// this says 1280 MiB for the layers that see everything and 300 MiB for
// the window, and llama-server said the same to the MiB.
func (s Shape) cacheBytes(ctx int) int64 {
	cells := min(ctx, s.Window+windowSpare)
	return perToken(s.FullLayers, s.FullKVHeads, s.FullHeadDim)*int64(ctx) +
		perToken(s.WindowLayers, s.WindowKVHeads, s.WindowHeadDim)*int64(cells)
}

// checkpointBytes is what the checkpoints of the window take, each the
// window's cache for Window tokens.
func (s Shape) checkpointBytes() int64 {
	return checkpoints * perToken(s.WindowLayers, s.WindowKVHeads, s.WindowHeadDim) * int64(s.Window)
}

// NeedsAt is the memory a search with a context of ctx tokens takes with
// this model loaded.
func (m LanguageModel) NeedsAt(ctx int) int64 {
	return m.Download + m.Shape.cacheBytes(ctx) + m.Shape.checkpointBytes() + workingAllowance
}

// LanguageModels is every model that can be installed, largest first.
//
// Four models from three houses. Which of them a given machine is offered
// is RecommendedFor below, and the smallest machine that is offered one at
// all has sixteen gigabytes.
//
// Every one is published by whoever made the model rather than quantised
// by somebody else afterwards, so what is fetched is what its maker meant
// to release. Nothing is redistributed, so each licence is between the
// user and its maker.
//
// The sizes and the checksums are read off the real files, through the
// Hugging Face API, not guessed. The first of these was guessed once and
// was a gigabyte out.
func LanguageModels() []LanguageModel {
	const hf = "https://huggingface.co/"
	models := []LanguageModel{{
		Name:  "gemma-4-26B_q4_0-it.gguf",
		Title: "Gemma 4 26B A4B",
		Maker: "Google",
		About: "Twenty six billion parameters with four of them used per token, " +
			"so it runs like a far smaller model. The largest of these.",
		Download: 14_439_363_584,
		Context:  262_144,
		URL:      hf + "google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf",
		SHA256:   "3eca3b8f6d7baf218a7dd6bba5fb59a56ee25fe2d567b6f5f589b4f697eca51d",
		// Gemma 4 26B A4B: 30 layers, 5 over the whole context with 2 heads of
		// 512, and 25 over the last 1024 tokens with 8 heads of 256.
		Shape: Shape{FullLayers: 5, FullKVHeads: 2, FullHeadDim: 512, WindowLayers: 25, WindowKVHeads: 8, WindowHeadDim: 256, Window: 1024},
	}, {
		Name:     "Qwen3-14B-Q4_K_M.gguf",
		Title:    "Qwen3 14B",
		Maker:    "Alibaba",
		About:    "Fourteen billion parameters, quantised to four bits.",
		Download: 9_001_752_960,
		Context:  40_960,
		URL:      hf + "Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf",
		SHA256:   "500a8806e85ee9c83f3ae08420295592451379b4f8cf2d0f41c15dffeb6b81f0",
		// Qwen3 14B: 40 layers, every one over the whole context, 8 heads of 128.
		Shape: Shape{FullLayers: 40, FullKVHeads: 8, FullHeadDim: 128},
	}, {
		Name:  "gemma-4-12b-it-qat-q4_0.gguf",
		Title: "Gemma 4 12B",
		Maker: "Google",
		About: "Twelve billion parameters, trained to be quantised to four bits " +
			"rather than cut down to them afterwards.",
		Download: 6_975_879_296,
		Context:  262_144,
		URL:      hf + "google/gemma-4-12B-it-qat-q4_0-gguf/resolve/main/gemma-4-12b-it-qat-q4_0.gguf",
		SHA256:   "93567e57a8fe10b23569b9d9ec38cd005deedf71e29477c421a4b83f418a538b",
		// Gemma 4 12B: 48 layers, 8 over the whole context with 1 head of 512,
		// and 40 over the last 1024 tokens with 8 heads of 256.
		Shape: Shape{FullLayers: 8, FullKVHeads: 1, FullHeadDim: 512, WindowLayers: 40, WindowKVHeads: 8, WindowHeadDim: 256, Window: 1024},
	}, {
		Name:  "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		Title: "Ministral 3 8B",
		Maker: "Mistral",
		About: "Eight billion parameters, quantised to four bits. The smallest " +
			"of these, for a machine with less to spare.",
		Download: 5_198_911_904,
		Context:  262_144,
		URL:      hf + "mistralai/Ministral-3-8B-Instruct-2512-GGUF/resolve/main/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		SHA256:   "33e7a72cf5e6e2cfc2f2847075acc013d68bba023e35310cef86b5cf8fdca761",
		// Ministral 3 8B: 34 layers, every one over the whole context, 8 heads
		// of 128.
		Shape: Shape{FullLayers: 34, FullKVHeads: 8, FullHeadDim: 128},
	}}
	for i := range models {
		models[i].Needs = models[i].NeedsAt(judgedContext)
	}
	return models
}

// RecommendedFor is the model to offer a machine with this much memory:
// the largest it can hold comfortably, or, where none of them is
// comfortable, the largest it can hold at all.
//
// Largest means the largest model, by its file, not the one that takes the
// most memory. Those used to be the same thing, because memory was worked
// out from the file. They are not: Ministral 3 8B takes more memory than
// Gemma 4 12B for a search of any length, and is the smaller model.
//
// A machine that will not say how much memory it has is offered the
// smallest, because the smallest is the one most likely to run, and a
// recommendation that cannot be checked should be the cautious one.
//
// It answers with nothing when nothing fits. Then the app says so and
// the Anthropic API is the way through, which is the whole reason there
// are two ways.
func RecommendedFor(total int64) (LanguageModel, bool) {
	models := LanguageModels()
	if len(models) == 0 {
		return LanguageModel{}, false
	}
	if total <= 0 {
		smallest := models[0]
		for _, m := range models {
			if m.Download < smallest.Download {
				smallest = m
			}
		}
		return smallest, true
	}
	var best LanguageModel
	var found bool
	for _, want := range []Fit{FitsWell, FitsTight} {
		for _, m := range models {
			if m.FitsIn(total) != want {
				continue
			}
			if !found || m.Download > best.Download {
				best, found = m, true
			}
		}
		if found {
			return best, true
		}
	}
	return LanguageModel{}, false
}

// LanguageModelByName finds one by the file it lands as.
func LanguageModelByName(name string) (LanguageModel, bool) {
	for _, m := range LanguageModels() {
		if m.Name == name {
			return m, true
		}
	}
	return LanguageModel{}, false
}

// Installed says whether this model is on the machine and is a model. A
// file of the right name is not enough: a download that stopped leaves one,
// and the app would then start a search that fails on the first prompt.
func (m LanguageModel) Installed(dir string) bool {
	if dir == "" {
		dir = ModelsDir()
	}
	path := filepath.Join(dir, m.Name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	// A model, and not a page saying no saved under a model's name. Not
	// the size: the size here is approximate, and a file judged against an
	// approximate size is a model that is there being called missing. A
	// download that stopped never gets this far anyway, because it lands
	// under a part name and is only moved once it is whole.
	return isGGUF(path)
}

// isGGUF says whether a file begins the way a model does. Four bytes, and
// they are the only thing that tells a model from an error page saved
// under a model's name.
func isGGUF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return false
	}
	return string(magic) == "GGUF"
}

// Fit is what a machine can do with a model.
type Fit string

const (
	// FitsWell means it runs with room to spare.
	FitsWell Fit = "fits"
	// FitsTight means it runs and the machine has little left. It will
	// work and everything else on the machine will feel it.
	FitsTight Fit = "tight"
	// TooBig means it does not fit in memory at all. Running it anyway
	// means swapping, which is minutes an answer rather than seconds.
	TooBig Fit = "too big"
	// FitUnknown means the machine did not say how much memory it has.
	// Then nothing is hidden and nothing is promised.
	FitUnknown Fit = "unknown"
)

// memoryHeadroom is how much a machine has to have left over for a model
// to be comfortable. A model is never the only thing running: the system,
// the app, its webview, a browser and whatever else is open. Below this the
// machine swaps, and a model that swaps is a model that takes minutes to
// answer.
const memoryHeadroom = 8 << 30

// FitsIn says what a machine with this much memory can do with the model.
func (m LanguageModel) FitsIn(total int64) Fit {
	switch {
	case total <= 0:
		return FitUnknown
	case m.Needs+memoryHeadroom <= total:
		return FitsWell
	case m.Needs <= total:
		return FitsTight
	default:
		return TooBig
	}
}

// MachineMemory is how much memory this machine has, in bytes, or zero
// when it will not say. Zero is an answer: it means nothing is hidden and
// nothing is promised, which is better than guessing on somebody's behalf.
func MachineMemory() int64 {
	switch runtime.GOOS {
	case "darwin":
		res := run(context.Background(), "", "sysctl", "-n", "hw.memsize")
		if res.Code == 0 {
			if n, err := strconv.ParseInt(strip(res.Stdout), 10, 64); err == nil {
				return n
			}
		}
	case "linux":
		return memTotal("/proc/meminfo")
	}
	return 0
}

// memTotal reads MemTotal out of a meminfo file, which is in kibibytes.
func memTotal(path string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := lines.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		n, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return n * 1024
	}
	return 0
}

// InstallLanguageModel fetches a model, reporting as it goes.
//
// Like the speech model it is written to be interrupted: the download lands
// under a part name and is only moved into place once it is whole, so a
// machine that loses power or a person who presses cancel is left with no
// model rather than half of one. Half a model is worse than none, because
// it looks installed.
func InstallLanguageModel(ctx context.Context, log *Log, m LanguageModel, dir string) error {
	if dir == "" {
		dir = ModelsDir()
	}
	if m.Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(dir, m.Name+".part")
	final := filepath.Join(dir, m.Name)

	// Bytes that turned out to be the wrong ones go, so a second try does
	// not carry on from them for ever. Bytes that simply stopped arriving
	// stay, because that is what makes the next try carry on rather than
	// begin again at nothing.
	wrong := func(err error) error {
		os.Remove(part)
		return err
	}

	return log.Step("language model", func() error {
		sum, err := download(ctx, log, m.URL, m.Download, part, m.Title)
		if err != nil {
			return err
		}
		if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
			return wrong(renderErr("%s did not arrive as expected. It should be %s and came to %s.",
				m.Title, m.SHA256, sum))
		}
		// Whether a checksum is pinned or not, what arrived has to be a
		// model. A page saying no, saved under a model's name, is the
		// thing this catches.
		if !isGGUF(part) {
			return wrong(renderErr("what came back from %s is not a model file.", m.URL))
		}
		log.ClearProgress()
		return os.Rename(part, final)
	})
}
