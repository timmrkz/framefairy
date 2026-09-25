package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Planning on this machine
//
// llama.cpp's server runs the model. The tool starts it as a helper program,
// the way it runs ffmpeg, sends one request and stops it again. The answer is
// held to a JSON schema while it is generated, so it always has the shape of
// a plan and no repair call is ever needed.
// ---------------------------------------------------------------------------

// LocalModel says how to reach a model on this machine.
type LocalModel struct {
	// Server is the llama-server binary.
	Server string
	// Model is the GGUF model file.
	Model string
	// URL is an already running llama-server. When set, nothing is started.
	URL string
	// Think is the most the model may think before it answers, in tokens.
	// Negative is no limit and 0 is no thinking at all.
	Think int
}

// DefaultThink is how much the local model may think before it answers.
//
// Left to itself, Gemma 4 thinks about a half hour window for 12,000 tokens
// or more, which on an M2 Max is four minutes before the first clip is
// written, and most of every search. 2,048 tokens is 45 seconds. When the
// budget runs out the model is told so and writes its answer.
const DefaultThink = 2048

// thinkEnough is what the model is told, at the end of its thoughts, when
// its budget runs out.
const thinkEnough = "\n\nThat is enough thinking. Time to write the answer.\n"

// LlamaServerPath decides which llama-server to run, the same way ffmpeg is
// decided: the one named in the environment, then the one sitting beside the
// program, then the search path. See ToolPath in tools.go.
//
// The middle step is the one that matters for a shipped app. A customer has
// no Homebrew and no terminal, so the only llama-server they will ever have
// is the one we put next to the program. Looking only on the search path is
// how a machine with a model on it still cannot find a clip.
func LlamaServerPath() string {
	return ToolPath("FRAMEFAIRY_LLAMA_SERVER", "llama-server")
}

// HasLlamaServer says whether a llama-server can be run at all. The setup
// and the settings ask this before offering the local way, because a model
// on its own is fifteen gigabytes that cannot answer anything.
func HasLlamaServer() bool {
	_, err := exec.LookPath(LlamaServerPath())
	return err == nil
}

// DefaultLocalModel finds the model file when none was named: the only
// .gguf file in ~/.framefairy/models.
func DefaultLocalModel() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".framefairy", "models")
	models := LocalModelFiles(dir)
	switch len(models) {
	case 1:
		return models[0], nil
	case 0:
		return "", renderErr("no language model found in %s. Download one as docs/INSTALL.md "+
			"describes, or name the file with --llm-model.", dir)
	}
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = filepath.Base(m)
	}
	return "", renderErr("several language models are in %s (%s). Choose one with --llm-model.",
		dir, strings.Join(names, ", "))
}

// LocalModelFiles is every model in a folder, in order: each .gguf, a
// model split in parts by its first part, and never the image input files
// some models ship beside them.
func LocalModelFiles(dir string) []string {
	found, _ := filepath.Glob(filepath.Join(dir, "*.gguf"))
	var models []string
	for _, f := range found {
		// Split models ship as name-00001-of-00003.gguf and are loaded
		// from their first part.
		base := filepath.Base(f)
		if strings.Contains(base, "-of-") && !strings.Contains(base, "-00001-of-") {
			continue
		}
		// Image input files ship next to some models and are not models.
		if strings.Contains(strings.ToLower(base), "mmproj") {
			continue
		}
		models = append(models, f)
	}
	sort.Strings(models)
	return models
}

// planSchema is the plan contract as a JSON schema. The property order is
// the order the model writes them in, which is why this is written out
// rather than built from a map.
func planSchema(lineCount, count int) string {
	return fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "clips": {
      "type": "array",
      "minItems": 1,
      "maxItems": %d,
      "items": {
        "type": "object",
        "properties": {
          "slug": {"type": "string", "pattern": "^[a-z0-9-]{1,64}$"},
          "title": {"type": "string", "minLength": 1, "maxLength": 200},
          "reason": {"type": "string", "minLength": 1, "maxLength": 300},
          "keep": {
            "type": "array",
            "minItems": 1,
            "maxItems": 12,
            "items": {
              "type": "array",
              "minItems": 2,
              "maxItems": 2,
              "items": {"type": "integer", "minimum": 1, "maximum": %d}
            }
          }
        },
        "required": ["slug", "title", "reason", "keep"],
        "additionalProperties": false
      }
    }
  },
  "required": ["clips"],
  "additionalProperties": false
}`, max(count, 1), max(lineCount, 1))
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// contextFor sizes the model's context to the prompt, with room for the
// answer. Memory use grows with it, so it is not simply set to the maximum.
func contextFor(promptChars, maxTokens int) int {
	need := int(float64(promptChars)/localCharsPerToken) + localAnswerRoom(maxTokens)
	size := 16384
	for size < need && size < contextCeiling {
		size *= 2
	}
	return size
}

var localClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// stopGrace is how long llama-server has to go by itself once it is asked
// to. Idle, it goes well inside it.
var stopGrace = 500 * time.Millisecond

// startServer runs llama-server and waits until the model is loaded.
func (e *Engine) startServer(ctx context.Context, m LocalModel, contextSize int,
	logDir string) (string, func(), error) {
	if _, err := os.Stat(m.Model); err != nil {
		return "", nil, renderErr("the language model %s cannot be read: %s", m.Model, err)
	}
	server := m.Server
	if server == "" {
		server = LlamaServerPath()
	}
	if _, err := exec.LookPath(server); err != nil {
		return "", nil, renderErr("%s was not found. Install llama.cpp as docs/INSTALL.md "+
			"describes, or point --llm-server at the binary.", server)
	}
	port, err := freePort()
	if err != nil {
		return "", nil, err
	}
	args := []string{"-m", m.Model, "--host", "127.0.0.1", "--port", itoa(port),
		"-c", itoa(contextSize), "-ngl", "999",
		// One ask at a time, with the whole context for it. It also keeps
		// what one ask read for the next, so a search that follows a
		// warm-up finds the start of its prompt already read. Left to
		// itself the server makes four slots and a sliding window cache
		// for each, which is memory nothing uses, and the memory a model
		// is judged by assumes one. See windowSpare in language.go.
		"-np", "1",
		// Level 4 is where llama.cpp says what it took from memory: the
		// cache, the working buffers, and the checkpoints of the window it
		// keeps in ordinary memory. Level 3, its own default, leaves all
		// of that out of the log. It adds a few lines an ask, not a line
		// a token.
		"-lv", "4"}
	e.Log.Detail("%s %s", server, strings.Join(args, " "))
	cmd := exec.Command(server, args...)
	var logFile *os.File
	if logDir != "" {
		// The model can be loaded ahead of a search, before anything else
		// of the episode has made its logs folder. Without the folder the
		// log was lost, and with it the only record of what the model
		// took from memory.
		if err = os.MkdirAll(logDir, 0o755); err == nil {
			logFile, err = os.Create(filepath.Join(logDir, "llm-server.log"))
		}
		if err != nil {
			e.Log.Warn("llama-server's output is not kept: %s", err)
		} else {
			cmd.Stdout, cmd.Stderr = logFile, logFile
		}
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return "", nil, renderErr("cannot start %s: %s", server, err)
	}
	// Written down while it runs, so a server an app that crashed left
	// behind is found and stopped the next time the app starts.
	noteServer(serverNote{PID: cmd.Process.Pid, Port: port, Model: m.Model})
	// exited is closed once the server has gone, and exitErr says how. Only
	// the goroutine that waits for it writes them, so stopping it never
	// reads the process's state while that goroutine writes it.
	exited := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = cmd.Wait()
		close(exited)
	}()
	var closeLog sync.Once
	stop := func() {
		select {
		case <-exited:
		default:
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-exited:
			case <-time.After(stopGrace):
				// llama-server can hang on the way out after an interrupt:
				// it joins the threads of its HTTP server, and one still
				// waiting on an answer it will never give keeps it there.
				// Its own source says so beside its signal handler, and
				// offers a second Ctrl+C to end it. It has nothing to save,
				// so it is killed. Waiting five seconds for it was the app
				// frozen on Cmd+Q and on removing an episode mid-search.
				_ = cmd.Process.Kill()
				<-exited
			}
		}
		closeLog.Do(func() {
			forgetServer(cmd.Process.Pid)
			if logFile != nil {
				logFile.Close()
			}
		})
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			stop()
			return "", nil, ctx.Err()
		case <-exited:
			stop()
			return "", nil, renderErr("%s stopped while loading the model (%v). Its output is "+
				"in %s", server, exitErr, filepath.Join(logDir, "llm-server.log"))
		case <-time.After(500 * time.Millisecond):
		}
		// How far the loading is, is the search's to say: it knows how long
		// it took before.
		response, err := localClient.Get(url + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				took := time.Since(started).Seconds()
				if logFile != nil {
					if before, ok := setupTime(logFile.Name()); ok {
						// What loading costs is worth knowing in two parts:
						// llama-server setting itself and the graphics up,
						// and reading the model into memory.
						e.Log.Info("model loaded in %ss, %ss of it before llama-server began reading the model",
							fixed(took, 1), fixed(before, 1))
						return url, stop, nil
					}
				}
				e.Log.Info("model loaded in %ss", fixed(took, 1))
				return url, stop, nil
			}
		}
		if time.Since(started) > 10*time.Minute {
			stop()
			return "", nil, renderErr("the language model did not finish loading within 10 minutes")
		}
	}
}

// setupTime reads from llama-server's log how long it took before it began
// reading the model file. Each line starts with the time since it started,
// minutes, seconds, milliseconds and microseconds, as in 0.19.335.706.
func setupTime(logPath string) (float64, bool) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "load_model: loading model") {
			continue
		}
		return serverStamp(line)
	}
	return 0, false
}

func serverStamp(line string) (float64, bool) {
	words := fields(line)
	if len(words) == 0 {
		return 0, false
	}
	parts := strings.Split(words[0], ".")
	if len(parts) != 4 {
		return 0, false
	}
	var n [4]int
	for i, part := range parts {
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 {
			return 0, false
		}
		n[i] = v
	}
	return float64(n[0])*60 + float64(n[1]) + float64(n[2])/1e3 + float64(n[3])/1e6, true
}

// chatError is what llama-server says when it refuses a request.
type chatError struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CallLocal asks the model on this machine for a plan. The answer is read
// as it is written, and listen hears it arrive. What comes back is the
// answer with how the model got to it: how long it read and thought and
// wrote.
func (e *Engine) CallLocal(ctx context.Context, m LocalModel, prompt string,
	lineCount, count, maxTokens int, logDir string, listen *Listener) (*localAnswer, error) {
	url := strings.TrimRight(m.URL, "/")
	if url == "" {
		// A model loaded while the transcript was still on its way is used
		// as it is. The search lets go of it when it is done, and it stops.
		size := localContextFor(m.Model, runeLen(prompt), maxTokens)
		if !modelReady(m.Model, size) {
			listen.part(partLoading)
		}
		held, release, err := e.holdModel(ctx, m, size, logDir)
		if err != nil {
			return nil, err
		}
		defer release(0)
		url = held
	}
	listen.part(partReading)

	schema := json.RawMessage(planSchema(lineCount, count))
	body, err := json.Marshal(map[string]any{
		"model": filepath.Base(m.Model),
		"messages": []map[string]string{
			{"role": "system", "content": SystemPrompt},
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens,
		// How long the model may think. The server stops the thought at the
		// budget and closes it with the message, so the answer follows.
		"reasoning_budget_tokens":  max(m.Think, -1),
		"reasoning_budget_message": thinkEnough,
		// Streamed, with the reading of the prompt reported as it goes and
		// what it cost at the end, which a stream otherwise leaves out.
		"stream":          true,
		"stream_options":  map[string]any{"include_usage": true},
		"return_progress": true,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "plan", "strict": true, "schema": schema,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if logDir != "" {
		_ = os.WriteFile(filepath.Join(logDir, "plan-prompt.txt"),
			[]byte("=== system ===\n"+SystemPrompt+"\n\n=== user ===\n"+prompt), 0o644)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("content-type", "application/json")

	started := time.Now()
	response, err := localClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, renderErr("the local model did not answer: %s", Scrub(err.Error(), 200))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		// A refusal is kept to read. An answer is kept in the saved reply,
		// with how the model got to it, so there is one file per answer.
		if logDir != "" {
			writeJSONText(filepath.Join(logDir, "plan-error.json"), raw)
		}
		message := fmt.Sprintf("status %d", response.StatusCode)
		var refused chatError
		if json.Unmarshal(raw, &refused) == nil && refused.Error != nil {
			message = refused.Error.Message
		}
		return nil, renderErr("the local model reported an error: %s", Scrub(message, 400))
	}
	answer, err := readLocalStream(response.Body, listen)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if strip(answer.Content) == "" {
		return nil, renderErr("the local model returned no answer")
	}
	reading, writing := 0.0, 0.0
	if t := answer.Timings; t != nil {
		reading, writing = t.PromptPerSecond, t.PredictedPerSecond
	}
	e.Log.Info("local model answered in %ss  read %s tok at %s tok/s  wrote %s tok at %s tok/s",
		fixed(time.Since(started).Seconds(), 1),
		commas(answer.PromptTokens), fixed(reading, 0),
		commas(answer.Written), fixed(writing, 1))
	if answer.Reasoning > 0 {
		e.Log.Detail("the model thought for %s characters before it answered",
			commas(answer.Reasoning))
	}
	if answer.FinishReason == "length" {
		e.Log.Warn("the answer hit the %s token ceiling. Raise --max-tokens if clips are missing.",
			commas(maxTokens))
	}
	return answer, nil
}
