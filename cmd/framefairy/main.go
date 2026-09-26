// Command framefairy turns a long-form podcast master into vertical short-form
// clips.
//
//	framefairy episode.mp4
//
// It picks the moments, condenses them, works out the vertical framing, burns
// in the captions and writes finished clips. ffmpeg does the video work.
package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"framefairy/asr"
	"framefairy/engine"
)

type kind int

const (
	kString kind = iota
	kInt
	kFloat
	kBool
	kList
)

type flagSpec struct {
	names   []string
	kind    kind
	metavar string
	help    string
	set     func(o *engine.Options, value string) error
}

func intValue(name string, into func(o *engine.Options, v int)) func(*engine.Options, string) error {
	return func(o *engine.Options, value string) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("argument %s: invalid int value: '%s'", name, value)
		}
		into(o, n)
		return nil
	}
}

func floatValue(name string, into func(o *engine.Options, v float64)) func(*engine.Options, string) error {
	return func(o *engine.Options, value string) error {
		t := strings.ToLower(strings.TrimSpace(value))
		var f float64
		switch t {
		case "inf", "+inf", "infinity":
			f = math.Inf(1)
		case "-inf", "-infinity":
			f = math.Inf(-1)
		case "nan":
			f = math.NaN()
		default:
			var err error
			if f, err = strconv.ParseFloat(t, 64); err != nil {
				return fmt.Errorf("argument %s: invalid float value: '%s'", name, value)
			}
		}
		into(o, f)
		return nil
	}
}

func specs() []flagSpec {
	d := engine.DefaultOptions()
	return []flagSpec{
		{[]string{"--clips"}, kString, "CLIPS", "use this clip plan instead of making one",
			func(o *engine.Options, v string) error { o.ClipsPath = v; return nil }},
		{[]string{"--replan"}, kBool, "", "discard the saved plan and choose again",
			func(o *engine.Options, _ string) error { o.Replan = true; return nil }},
		{[]string{"--plan-only"}, kBool, "", "write clips.json and stop, without rendering",
			func(o *engine.Options, _ string) error { o.PlanOnly = true; return nil }},
		{[]string{"--transcribe-only"}, kBool, "", "transcribe the audio, write a readable " +
			"transcript to the logs folder and stop. Makes no API call",
			func(o *engine.Options, _ string) error { o.TranscribeOnly = true; return nil }},
		{[]string{"--asr-model"}, kString, "DIR", "folder holding the speech model (default " +
			"~/.framefairy/models/" + engine.ModelName + ")",
			func(o *engine.Options, v string) error { o.ASRModel = v; return nil }},
		{[]string{"--training-dir"}, kString, "DIR", "folder for the training records of every " +
			"episode (default ~/.framefairy/training, or FRAMEFAIRY_TRAINING)",
			func(o *engine.Options, v string) error { o.TrainingDir = v; return nil }},
		{[]string{"--count"}, kInt, "COUNT", fmt.Sprintf("how many clips to look for (default %d)", d.Count),
			intValue("--count", func(o *engine.Options, v int) { o.Count = v })},
		{[]string{"--min"}, kFloat, "MIN", "shortest acceptable clip, in seconds (default 20)",
			floatValue("--min", func(o *engine.Options, v float64) { o.Min = v })},
		{[]string{"--max"}, kFloat, "MAX", "target ceiling for a clip, in seconds (default 30). " +
			"A clip that runs a little over is fine",
			floatValue("--max", func(o *engine.Options, v float64) { o.Max = v })},
		{[]string{"--from"}, kString, "START_AT", "only consider the episode from this point, as " +
			"seconds or a timecode like 1:00:00",
			func(o *engine.Options, v string) error { o.From = v; return nil }},
		{[]string{"--to"}, kString, "END_AT", "only consider the episode up to this point",
			func(o *engine.Options, v string) error { o.To = v; return nil }},
		{[]string{"--max-pause"}, kFloat, "MAX_PAUSE", "cut any silence longer than this even when " +
			"the model chose to keep it. Off by default, because which pauses survive is the model's call",
			floatValue("--max-pause", func(o *engine.Options, v float64) { o.MaxPause = &v })},
		{[]string{"--keep-pause"}, kFloat, "KEEP_PAUSE", "how much of a cut silence to leave on each " +
			"side, so the join still breathes (default 0.10 s)",
			floatValue("--keep-pause", func(o *engine.Options, v float64) { o.KeepPause = v })},
		{[]string{"--silence-db"}, kFloat, "SILENCE_DB", "what counts as silence, in dB. Measured from " +
			"the audio when not given. Lower treats more quiet sound as speech, for instance -50",
			floatValue("--silence-db", func(o *engine.Options, v float64) { o.SilenceDB = &v })},
		{[]string{"--context"}, kString, "CONTEXT", "guest name, company, vocabulary. Improves both " +
			"the choice of moments and proper nouns",
			func(o *engine.Options, v string) error { o.Context = v; return nil }},
		{[]string{"--recipe"}, kString, "RECIPE", "how the model is asked for clips: " +
			strings.Join(engine.RecipeNames(), ", ") + " (default " + engine.DefaultRecipe + "). " +
			"Answers to another recipe are kept apart and never recorded for training",
			func(o *engine.Options, v string) error {
				if _, err := engine.RecipeNamed(v); err != nil {
					return err
				}
				o.Recipe = v
				return nil
			}},
		{[]string{"--compare"}, kString, "RECIPES", "search the window once with each recipe, as " +
			"experiments, and write a report of what each cost and found beside the plans in " +
			"<episode>.framefairy/experiments, for instance --compare lines,stories. A recipe with " +
			"@ and a number thinks that many tokens, so --compare stories,stories@1024 compares " +
			"thinking budgets. Nothing is rendered and the episode's own plan is left alone",
			func(o *engine.Options, v string) error {
				for _, name := range strings.Split(v, ",") {
					name = strings.TrimSpace(name)
					if _, err := engine.ParseVariant(name); err != nil {
						return err
					}
					o.Compare = append(o.Compare, name)
				}
				return nil
			}},
		{[]string{"--planner"}, kString, "PLANNER", "local plans on this machine with llama.cpp, api uses " +
			"the Claude API (default local)",
			func(o *engine.Options, v string) error { o.Planner = v; return nil }},
		{[]string{"--llm-model"}, kString, "FILE", "the GGUF model file for local planning (default: " +
			"the only .gguf file in ~/.framefairy/models). A bare file name is looked for there too",
			func(o *engine.Options, v string) error { o.LLMModel = engine.LocalModelPath(v); return nil }},
		{[]string{"--llm-server"}, kString, "PATH", "the llama-server binary (default llama-server on PATH)",
			func(o *engine.Options, v string) error { o.LLMServer = v; return nil }},
		{[]string{"--llm-url"}, kString, "URL", "use an already running llama-server, for instance " +
			"http://127.0.0.1:8080, instead of starting one",
			func(o *engine.Options, v string) error { o.LLMURL = v; return nil }},
		{[]string{"--model"}, kString, "MODEL", "which Claude model plans with --planner api (default " + d.Model + ")",
			func(o *engine.Options, v string) error { o.Model = v; return nil }},
		{[]string{"--max-tokens"}, kInt, "MAX_TOKENS", "ceiling on the model's reply length. This is " +
			"a limit, not a charge. Current models think before they answer and those tokens come " +
			"out of the same ceiling, so it needs headroom (default 48000)",
			intValue("--max-tokens", func(o *engine.Options, v int) { o.MaxTokens = v })},
		{[]string{"--think"}, kInt, "TOKENS", "how long the local model may think before it " +
			"answers, in tokens. -1 is no limit, 0 is no thinking (default 2048)",
			intValue("--think", func(o *engine.Options, v int) { o.Think = v })},
		{[]string{"--ffmpeg"}, kString, "FFMPEG", "path to an ffmpeg built with libass, if the one on " +
			"PATH is not",
			func(o *engine.Options, v string) error { o.FFmpeg = v; return nil }},
		{[]string{"--ffprobe"}, kString, "FFPROBE", "path to ffprobe, found next to --ffmpeg when not given",
			func(o *engine.Options, v string) error { o.FFprobe = v; return nil }},
		{[]string{"--no-captions"}, kBool, "", "render without burning in captions. The per-clip srt " +
			"files are still written",
			func(o *engine.Options, _ string) error { o.NoCaptions = true; return nil }},
		{[]string{"--prefill"}, kBool, "", "start the model's reply with an opening brace. Models " +
			"with thinking enabled reject this",
			func(o *engine.Options, _ string) error { o.Prefill = true; return nil }},
		{[]string{"--budget"}, kFloat, "BUDGET", "refuse to send a request estimated to cost more " +
			"than this many dollars (default 2.00)",
			floatValue("--budget", func(o *engine.Options, v float64) { o.Budget = v })},
		{[]string{"--verbose", "-v"}, kBool, "", "show every ffmpeg command and API detail", nil},
		{[]string{"--no-colour", "--no-color"}, kBool, "", "plain output", nil},
		{[]string{"--events"}, kString, "FILE", "also write every step and progress update to FILE " +
			"as JSON lines, for testing the app's event feed", nil},
		{[]string{"--clip"}, kList, "CLIP", "render only this clip id, repeatable",
			func(o *engine.Options, v string) error { o.Clip = append(o.Clip, v); return nil }},
		{[]string{"--out"}, kString, "OUT", "output directory, defaults to <source>.framefairy/out",
			func(o *engine.Options, v string) error { o.Out = v; return nil }},
		{[]string{"--width"}, kInt, "WIDTH", "output width (default 1080)",
			intValue("--width", func(o *engine.Options, v int) { o.Width = v })},
		{[]string{"--height"}, kInt, "HEIGHT", "output height (default 1920)",
			intValue("--height", func(o *engine.Options, v int) { o.Height = v })},
		{[]string{"--no-upscale"}, kBool, "", "keep the native crop size instead of scaling to the " +
			"output size, so every output pixel is a source pixel",
			func(o *engine.Options, _ string) error { o.NoUpscale = true; return nil }},
		{[]string{"--crf"}, kInt, "CRF", "quality, lower is better (default 18)",
			intValue("--crf", func(o *engine.Options, v int) { o.CRF = v })},
		{[]string{"--preset"}, kString, "PRESET", "x264 speed against compression (default slow). " +
			"Only libx264 has presets, and it is ignored by any other encoder",
			func(o *engine.Options, v string) error { o.Preset = v; return nil }},
		{[]string{"--encoder"}, kString, "ENCODER", "the video encoder to use, instead of the best " +
			"one this ffmpeg has (" + engine.EncoderNames() + ")",
			func(o *engine.Options, v string) error { o.Encoder = v; return nil }},
		{[]string{"--audio-bitrate"}, kString, "AUDIO_BITRATE", "aac bitrate (default 256k)",
			func(o *engine.Options, v string) error { o.AudioBitrate = v; return nil }},
		{[]string{"--preview"}, kBool, "", "fast low quality render for checking cuts",
			func(o *engine.Options, _ string) error { o.Preview = true; return nil }},
		{[]string{"--font"}, kString, "FONT", "caption font, must be installed on the system",
			func(o *engine.Options, v string) error { o.Font = v; return nil }},
		{[]string{"--font-size"}, kInt, "FONT_SIZE", "caption size, in pixels of a 1080x1920 frame",
			intValue("--font-size", func(o *engine.Options, v int) { o.FontSize = v })},
		{[]string{"--margin-v"}, kInt, "MARGIN_V", "caption distance from the bottom edge, in pixels " +
			"of a 1080x1920 frame",
			intValue("--margin-v", func(o *engine.Options, v int) { o.MarginV = &v })},
		{[]string{"--highlight-colour", "--highlight-color"}, kString, "COLOUR", "colour of the " +
			"pill behind the word being spoken, as #RRGGBB (default #942192)",
			func(o *engine.Options, v string) error { o.HighlightColour = v; return nil }},
		{[]string{"--no-highlight"}, kBool, "", "plain captions, without the bouncing word highlight",
			func(o *engine.Options, _ string) error { o.NoHighlight = true; return nil }},
		{[]string{"--refresh-captions"}, kBool, "", "rebuild per-clip caption files from the " +
			"transcript, discarding manual corrections",
			func(o *engine.Options, _ string) error { o.RefreshCaptions = true; return nil }},
		{[]string{"--dry-run"}, kBool, "", "print the ffmpeg commands instead of running them",
			func(o *engine.Options, _ string) error { o.DryRun = true; return nil }},
	}
}

const usageLine = "usage: framefairy [-h] [--version] [options] source"

func printHelp(w io.Writer) {
	fmt.Fprintln(w, usageLine)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Turn a long-form episode into finished vertical clips.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "positional arguments:")
	fmt.Fprintln(w, "  source                the graded master mp4")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  -h, --help            show this help message and exit")
	fmt.Fprintln(w, "  --version             show the version and exit")
	for _, s := range specs() {
		left := "  " + strings.Join(s.names, ", ")
		if s.metavar != "" {
			left += " " + s.metavar
		}
		if len(left) >= 24 {
			fmt.Fprintln(w, left)
			left = ""
		}
		fmt.Fprintf(w, "%-24s%s\n", left, wrap(s.help, 54, strings.Repeat(" ", 24)))
	}
}

func wrap(text string, width int, indent string) string {
	words := strings.Fields(text)
	var lines []string
	current := ""
	for _, w := range words {
		if current != "" && len(current)+1+len(w) > width {
			lines = append(lines, current)
			current = w
		} else if current == "" {
			current = w
		} else {
			current += " " + w
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n"+indent)
}

type parsed struct {
	opts    engine.Options
	verbose bool
	plain   bool
	events  string
	help    bool
	version bool
}

func usageError(msg string) error {
	return fmt.Errorf("%s\nframefairy: error: %s", usageLine, msg)
}

// parseArgs follows argparse's rules, so every command that worked with the
// Python version works here. Options and the source may come in any order,
// values may be joined with "=", and a unique prefix of an option is enough.
func parseArgs(argv []string) (parsed, error) {
	p := parsed{opts: engine.DefaultOptions()}
	all := specs()
	byName := map[string]*flagSpec{}
	var longNames []string
	for i := range all {
		for _, n := range all[i].names {
			byName[n] = &all[i]
			if strings.HasPrefix(n, "--") {
				longNames = append(longNames, n)
			}
		}
	}
	byName["--help"], byName["-h"], byName["--version"] = nil, nil, nil
	longNames = append(longNames, "--help", "--version")
	sort.Strings(longNames)

	lookup := func(name string) (string, *flagSpec, error) {
		if spec, ok := byName[name]; ok {
			return name, spec, nil
		}
		if !strings.HasPrefix(name, "--") {
			return "", nil, usageError("unrecognized arguments: " + name)
		}
		var matches []string
		for _, n := range longNames {
			if strings.HasPrefix(n, name) {
				matches = append(matches, n)
			}
		}
		switch len(matches) {
		case 1:
			return matches[0], byName[matches[0]], nil
		case 0:
			return "", nil, usageError("unrecognized arguments: " + name)
		}
		return "", nil, usageError(fmt.Sprintf("ambiguous option: %s could match %s",
			name, strings.Join(matches, ", ")))
	}

	var positional []string
	onlyPositional := false
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if onlyPositional || arg == "-" || !strings.HasPrefix(arg, "-") || isNumber(arg) {
			positional = append(positional, arg)
			continue
		}
		if arg == "--" {
			onlyPositional = true
			continue
		}
		name, value, joined := strings.Cut(arg, "=")
		canonical, spec, err := lookup(name)
		if err != nil {
			return p, err
		}
		switch canonical {
		case "--help", "-h":
			p.help = true
			continue
		case "--version":
			p.version = true
			continue
		case "--verbose", "-v":
			p.verbose = true
			continue
		case "--no-colour", "--no-color":
			p.plain = true
			continue
		}
		if spec.kind == kBool {
			if joined {
				return p, usageError(fmt.Sprintf("argument %s: ignored explicit argument '%s'",
					canonical, value))
			}
			_ = spec.set(&p.opts, "")
			continue
		}
		if !joined {
			if i+1 >= len(argv) || (strings.HasPrefix(argv[i+1], "-") && !isNumber(argv[i+1])) {
				return p, usageError(fmt.Sprintf("argument %s: expected one argument", canonical))
			}
			i++
			value = argv[i]
		}
		if canonical == "--events" {
			p.events = value
			continue
		}
		if err := spec.set(&p.opts, value); err != nil {
			return p, usageError(err.Error())
		}
	}
	if p.help || p.version {
		return p, nil
	}
	switch len(positional) {
	case 0:
		return p, usageError("the following arguments are required: source")
	case 1:
		p.opts.Source = positional[0]
	default:
		return p, usageError("unrecognized arguments: " + strings.Join(positional[1:], " "))
	}
	return p, nil
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func main() {
	argv := os.Args[1:]
	// "framefairy render ..." and "framefairy ..." both work, so the verb is optional
	// and stays available for the stages that come later.
	if len(argv) > 0 && argv[0] == "render" {
		argv = argv[1:]
	}
	p, err := parseArgs(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if p.help {
		printHelp(os.Stdout)
		return
	}
	if p.version {
		fmt.Println("framefairy " + engine.Version)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := engine.NewLog(os.Stdout, !p.plain, p.verbose)
	e := engine.NewEngine(log)
	e.OpenRecognizer = asr.Open
	var events *os.File
	if p.events != "" {
		events, err = os.Create(p.events)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot write %s: %s\n", p.events, err)
			os.Exit(1)
		}
		log.SetSink(engine.JSONLines(events))
	}
	var code int
	if len(p.opts.Compare) > 0 {
		// A comparison asks afresh, because what a search costs is half
		// of what is compared, and a saved answer costs nothing.
		p.opts.Replan = true
		_, report, err := e.Compare(ctx, p.opts, p.opts.Compare)
		if err != nil {
			log.Error("%s", err)
			code = 1
		} else {
			log.OK("the comparison is in %s", report)
		}
	} else {
		code = e.Run(ctx, p.opts)
	}
	stop()
	if events != nil {
		// os.Exit skips deferred calls, so the file is closed here.
		log.SetSink(nil)
		_ = events.Close()
	}
	os.Exit(code)
}
