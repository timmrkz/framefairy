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
	// Title is what it is called in the window, Maker who made it. They
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
	// Needs is the memory it takes to run, with the context the engine
	// asks for, in bytes. More than the file: the weights are in memory
	// and the context is on top of them.
	Needs int64 `json:"needs"`
	// URL is where it comes from, whoever published it rather than us. Nothing
	// is redistributed, so its licence is between the user and whoever
	// published it.
	URL string `json:"url"`
	// SHA256 of the file, where it is pinned. Empty means it is not, and
	// then the check is that what arrived is a model at all rather than
	// that it is exactly the one expected.
	SHA256 string `json:"-"`
}

// needs is what a model takes to run, worked out from what it takes on
// disk. The weights are the file, and the context and the runtime sit on
// top of them: a quarter more, which is where the eighteen gigabytes this
// project has always quoted for the fourteen gigabyte Gemma comes from.
//
// It is a rule rather than a measurement, and it is a rule because a
// measurement would have to be taken on every machine for every model. It
// is used to say whether a machine can hold a model, and for that being
// roughly right and never optimistic is what matters.
func needs(file int64) int64 { return file + file/4 }

// LanguageModels is every model that can be installed, largest first.
//
// Four models from three houses, across the range of machines somebody
// might have: the largest wants a machine with plenty and the smallest
// runs on one with sixteen gigabytes. Which of them a given machine is
// offered is RecommendedFor below.
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
	return []LanguageModel{{
		Name:  "gemma-4-26B_q4_0-it.gguf",
		Title: "Gemma 4 26B A4B",
		Maker: "Google",
		About: "Twenty six billion parameters with four of them used per token, " +
			"so it runs like a far smaller model. The largest of these.",
		Download: 14_439_363_584,
		Needs:    needs(14_439_363_584),
		URL:      hf + "google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf",
		SHA256:   "3eca3b8f6d7baf218a7dd6bba5fb59a56ee25fe2d567b6f5f589b4f697eca51d",
	}, {
		Name:     "Qwen3-14B-Q4_K_M.gguf",
		Title:    "Qwen3 14B",
		Maker:    "Alibaba",
		About:    "Fourteen billion parameters, quantised to four bits.",
		Download: 9_001_752_960,
		Needs:    needs(9_001_752_960),
		URL:      hf + "Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf",
		SHA256:   "500a8806e85ee9c83f3ae08420295592451379b4f8cf2d0f41c15dffeb6b81f0",
	}, {
		Name:  "gemma-4-12b-it-qat-q4_0.gguf",
		Title: "Gemma 4 12B",
		Maker: "Google",
		About: "Twelve billion parameters, trained to be quantised to four bits " +
			"rather than cut down to them afterwards.",
		Download: 6_975_879_296,
		Needs:    needs(6_975_879_296),
		URL:      hf + "google/gemma-4-12B-it-qat-q4_0-gguf/resolve/main/gemma-4-12b-it-qat-q4_0.gguf",
		SHA256:   "93567e57a8fe10b23569b9d9ec38cd005deedf71e29477c421a4b83f418a538b",
	}, {
		Name:  "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		Title: "Ministral 3 8B",
		Maker: "Mistral",
		About: "Eight billion parameters, quantised to four bits. The smallest " +
			"of these, for a machine with less to spare.",
		Download: 5_198_911_904,
		Needs:    needs(5_198_911_904),
		URL:      hf + "mistralai/Ministral-3-8B-Instruct-2512-GGUF/resolve/main/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		SHA256:   "33e7a72cf5e6e2cfc2f2847075acc013d68bba023e35310cef86b5cf8fdca761",
	}}
}

// RecommendedFor is the model to offer a machine with this much memory:
// the largest it can hold comfortably, or, where none of them is
// comfortable, the largest it can hold at all.
//
// A machine that will not say how much memory it has is offered the
// smallest, because the smallest is the one most likely to run, and a
// recommendation that cannot be checked should be the cautious one.
//
// It answers with nothing when nothing fits. Then the window says so and
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
			if m.Needs < smallest.Needs {
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
			if !found || m.Needs > best.Needs {
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
