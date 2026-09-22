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
	"strings"
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
}

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
	// German runs at roughly two and a half characters per token.
	need := promptChars*10/25 + min(maxTokens, 16384) + 2048
	size := 16384
	for size < need && size < 262144 {
		size *= 2
	}
	return size
}

var localClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

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
		"-c", itoa(contextSize), "-ngl", "999"}
	e.Log.Detail("%s %s", server, strings.Join(args, " "))
	cmd := exec.Command(server, args...)
	var logFile *os.File
	if logDir != "" {
		if logFile, err = os.Create(filepath.Join(logDir, "llm-server.log")); err == nil {
			cmd.Stdout, cmd.Stderr = logFile, logFile
		}
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return "", nil, renderErr("cannot start %s: %s", server, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	stop := func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-exited
			}
		}
		if logFile != nil {
			logFile.Close()
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			stop()
			return "", nil, ctx.Err()
		case err := <-exited:
			if logFile != nil {
				logFile.Close()
			}
			return "", nil, renderErr("%s stopped while loading the model (%v). Its output is "+
				"in %s", server, err, filepath.Join(logDir, "llm-server.log"))
		case <-time.After(500 * time.Millisecond):
		}
		// How far the loading is, is the search's to say: it knows how long
		// it took before.
		response, err := localClient.Get(url + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				e.Log.Detail("model loaded in %ss", fixed(time.Since(started).Seconds(), 1))
				return url, stop, nil
			}
		}
		if time.Since(started) > 10*time.Minute {
			stop()
			return "", nil, renderErr("the language model did not finish loading within 10 minutes")
		}
	}
}

// chatError is what llama-server says when it refuses a request.
type chatError struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CallLocal asks the model on this machine for a plan. The answer is read
// as it is written, and listen hears it arrive.
func (e *Engine) CallLocal(ctx context.Context, m LocalModel, prompt string,
	lineCount, count, maxTokens int, logDir string, listen *Listener) (string, error) {
	url := strings.TrimRight(m.URL, "/")
	if url == "" {
		listen.part(partLoading)
		started, stop, err := e.startServer(ctx, m, contextFor(runeLen(prompt), maxTokens), logDir)
		if err != nil {
			return "", err
		}
		defer stop()
		url = started
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
		return "", err
	}
	if logDir != "" {
		_ = os.WriteFile(filepath.Join(logDir, "plan-prompt.txt"),
			[]byte("=== system ===\n"+SystemPrompt+"\n\n=== user ===\n"+prompt), 0o644)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("content-type", "application/json")

	started := time.Now()
	response, err := localClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", renderErr("the local model did not answer: %s", Scrub(err.Error(), 200))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if logDir != "" {
			writeJSONText(filepath.Join(logDir, "plan-response.json"), raw)
		}
		message := fmt.Sprintf("status %d", response.StatusCode)
		var refused chatError
		if json.Unmarshal(raw, &refused) == nil && refused.Error != nil {
			message = refused.Error.Message
		}
		return "", renderErr("the local model reported an error: %s", Scrub(message, 400))
	}
	answer, err := readLocalStream(response.Body, listen)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	if logDir != "" {
		if kept, err := json.Marshal(answer); err == nil {
			writeJSONText(filepath.Join(logDir, "plan-response.json"), kept)
		}
	}
	if strip(answer.Content) == "" {
		return "", renderErr("the local model returned no answer")
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
	return answer.Content, nil
}
