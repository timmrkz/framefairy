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
	// Download is about how big the file is, in bytes. It is said before
	// anything starts, because a download nobody agreed to is a download
	// nobody wanted, and it is only ever used to say so and to report how
	// far a download has come when the server does not say. Nothing
	// depends on it being exact, because a figure that has not been read
	// off the real file is not exact.
	Download int64 `json:"download"`
	// Needs is the memory it takes to run, with the context the engine
	// asks for, in bytes. More than the file: the weights are in memory
	// and the context is on top of them.
	Needs int64 `json:"needs"`
	// URL is where it comes from, its own home rather than ours. Nothing
	// is redistributed, so its licence is between the user and whoever
	// published it.
	URL string `json:"url"`
	// SHA256 of the file, where it is pinned. Empty means it is not, and
	// then the check is that what arrived is a model at all rather than
	// that it is exactly the one expected.
	SHA256 string `json:"-"`
	// Recommended marks the one to offer on a machine that can hold it.
	Recommended bool `json:"recommended"`
}

// LanguageModels is every model that can be installed, largest first.
//
// There is one here and the shape is written for many, because the moment
// there are two the window has a real choice to put to somebody and the
// memory below decides which of them it may offer.
//
// A word about how short this list is. Every entry needs a URL that is
// really there, a size that is really that and, best of all, a checksum
// taken from the real file. The one below has been downloaded and run. The
// others cannot be added from a cloud session, because huggingface.co is
// not reachable from one, and a list of models that 404 is worse than a
// list of one that works.
func LanguageModels() []LanguageModel {
	return []LanguageModel{{
		Name:  "gemma-4-26B_q4_0-it.gguf",
		Title: "Gemma 4 26B A4B",
		Maker: "Google",
		About: "Quantised to four bits, and only four billion of its " +
			"twenty six are used per token, so it answers like a much " +
			"smaller model.",
		// Both of these are the figures scripts/models.sh has carried
		// since this model was first used, in bytes: 14.4 GiB to fetch
		// and about 18 GiB to run. Neither has been read off the file by
		// anything here, so neither is exact and nothing depends on it.
		Download:    15_461_882_265,
		Needs:       19_327_352_832,
		URL:         "https://huggingface.co/google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf",
		Recommended: true,
	}}
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
	defer os.Remove(part)

	return log.Step("language model", func() error {
		sum, err := download(ctx, log, m.URL, m.Download, part, m.Title)
		if err != nil {
			return err
		}
		if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
			return renderErr("%s did not arrive as expected. It should be %s and came to %s.",
				m.Title, m.SHA256, sum)
		}
		// Whether a checksum is pinned or not, what arrived has to be a
		// model. A page saying no, saved under a model's name, is the
		// thing this catches.
		if !isGGUF(part) {
			return renderErr("what came back from %s is not a model file.", m.URL)
		}
		log.ClearProgress()
		return os.Rename(part, final)
	})
}
