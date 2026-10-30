package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/runes"
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
	// Keep is how long a model this ask loaded stays loaded after it, for
	// an ask that follows at once and shares the start of this one. Zero
	// lets it go the moment the answer is in.
	Keep time.Duration
	// Seed and Temperature, see Options.
	Seed        int
	Temperature *float64
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
// decided: the one named in the environment, or else the one beside the
// program, checked. See FindTool in tools.go. Empty when there is none
// that may run, and FindLlamaServer says why.
func LlamaServerPath() string {
	return ToolPath("FRAMEFAIRY_LLAMA_SERVER", "llama-server")
}

// serverToRun is the llama-server an ask runs: the one named on the command
// line with --llm-server, which is somebody's own choice, or else the
// program's own, checked.
func serverToRun(named string) (string, error) {
	if named == "" {
		return FindLlamaServer()
	}
	if _, err := exec.LookPath(named); err != nil {
		return "", renderErr("%s was not found. Point --llm-server at the binary.", named)
	}
	return named, nil
}

// FindLlamaServer is LlamaServerPath with the reason when there is none.
func FindLlamaServer() (string, error) {
	return FindTool("FRAMEFAIRY_LLAMA_SERVER", "llama-server")
}

// HasLlamaServer says whether a llama-server can be run at all. The setup
// and the settings ask this before offering the local way, because a model
// on its own is fifteen gigabytes that cannot answer anything.
func HasLlamaServer() bool {
	server, err := FindLlamaServer()
	if err != nil {
		return false
	}
	// One named in the environment is taken as it stands, so it is looked
	// at here, the way starting it would.
	_, err = exec.LookPath(server)
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

// LocalModelPath is the model file a name given on the command line means.
// A file that is there as named is that file. A bare file name that is not
// is looked for in ~/.framefairy/models, where the models live, so the name
// the setup shows is enough.
func LocalModelPath(named string) string {
	if named == "" || filepath.Base(named) != named {
		return named
	}
	if _, err := os.Stat(named); err == nil {
		return named
	}
	if inModels := filepath.Join(ModelsDir(), named); isFile(disk, inModels) {
		return inModels
	}
	return named
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

// localStall is how long llama-server may send nothing before the ask is
// given up on. It speaks all the time while it works: a word of how far it
// has read the prompt after every batch of it, promptBatch tokens, then
// every token of its thought and of its answer. So two minutes of silence is a server that hangs,
// and a search waiting on it waits for good, with every video's
// transcription held up behind it, see lanes.go in the app. The client has
// no timeout of its own, because a whole answer takes minutes. Plan row
// 2.224. A variable so a test can make it short.
var localStall = 2 * time.Minute

// promptBatch is how many tokens of the prompt llama-server reads before
// it says how far it has come, its logical batch, `-b`. Its own default is
// 2048, and a machine that reads fewer than 17 tokens a second, the
// processor of a Windows or Linux machine without a graphics chip
// llama.cpp can use, would say nothing for longer than localStall and be
// stopped while it was well. With 512 it has to read fewer than 4.3 a
// second for that. It is also llama.cpp's physical batch, `-ub`, so the graphics
// chip of a Mac reads the prompt in the same pieces as before and no
// slower. Checked against llama.cpp b11430: the server sends its progress
// in post_decode in tools/server/server-context.cpp, once for every
// logical batch it decodes.
const promptBatch = 512

// slowestReading is the fewest tokens of the prompt a second a machine the
// app runs on may read without being taken for a stalled model. A
// processor of the last ten years is expected to read a model of a few
// billion active parameters, like Gemma 4 26B A4B, in tens of tokens a
// second, not measured here.
const slowestReading = 5.0

// stallWatch calls its function once nothing has been read for a while.
type stallWatch struct {
	*time.Timer
	after time.Duration
}

func watchStall(after time.Duration, gaveUp func()) *stallWatch {
	return &stallWatch{Timer: time.AfterFunc(after, gaveUp), after: after}
}

// reader is r, and every read that brings something starts the wait again.
func (w *stallWatch) reader(r io.Reader) io.Reader {
	return readerFunc(func(p []byte) (int, error) {
		n, err := r.Read(p)
		if n > 0 {
			w.Reset(w.after)
		}
		return n, err
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

var localClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// A llama-server the app starts answers only to the app. Left to itself
// it answers anybody: any program on the machine, and any web page open
// in a browser, because it allows every origin by default, and it lists
// what it is working on, which is the transcript of the episode being
// searched. Its port is on the loopback and different every time, but a
// page can look through every port in a few seconds.
//
// So each server gets a key of its own, made when it starts and handed
// over in its environment rather than on its command line, where every
// program on the machine could read it, and the list of what it is
// working on is switched off. The key lives as long as the server, by
// its address, and only goes with a request to that address. An address
// given by hand, a server somebody else started, gets no key.
var serverKeys sync.Map

func newServerKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// authorize puts the key of the server at url on a request to it.
func authorize(request *http.Request, url string) {
	if key, ok := serverKeys.Load(url); ok {
		request.Header.Set("Authorization", "Bearer "+key.(string))
	}
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
	if err := CheckModelFile(m.Model, ""); err != nil {
		return "", nil, err
	}
	// A model that does not fit in the memory free now is not started:
	// macOS would kill it while it loads, see freemem.go.
	if err := roomForModel(m.Model, contextSize); err != nil {
		return "", nil, err
	}
	server, err := serverToRun(m.Server)
	if err != nil {
		return "", nil, err
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
		"-lv", "4",
		// Nothing reads what the server is working on, and it is the
		// transcript, see serverKeys.
		"--no-slots",
		// How far it has read the prompt is said once a batch, see
		// promptBatch.
		"-b", itoa(promptBatch),
		// No more copies of the window than the memory a model is judged
		// by holds, see checkpoints in language.go. The transcript read
		// ahead of a search, see readahead.go, makes two with every read,
		// and llama.cpp keeps 32 of them by itself, gigabytes of them.
		"--ctx-checkpoints", itoa(checkpoints)}
	e.Log.Detail("%s %s", server, strings.Join(args, " "))
	key, err := newServerKey()
	if err != nil {
		return "", nil, renderErr("no key could be made for %s: %s", server, err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(server, args...)
	cmd.Env = append(os.Environ(), "LLAMA_API_KEY="+key)
	// The server says when it is up, see readyMarks, and the wait for it
	// ends on that line rather than on the next look of a clock.
	ready := make(chan struct{}, 1)
	heard := &hearReady{out: io.Discard, ready: ready}
	cmd.Stdout, cmd.Stderr = heard, heard
	var logFile *os.File
	if logDir != "" {
		// The model can be loaded ahead of a search, before anything else
		// of the episode has made its logs folder. Without the folder the
		// log was lost, and with it the only record of what the model
		// took from memory. The log is llama-server's own output, so it
		// is on the disk, see files.go.
		if err = os.MkdirAll(logDir, 0o755); err == nil {
			logFile, err = os.Create(filepath.Join(logDir, "llm-server.log"))
		}
		if err != nil {
			e.Log.Warn("llama-server's output is not kept: %s", err)
		} else {
			heard.out = logFile
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
	serverKeys.Store(url, key)
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
			case <-leaving(stopGrace):
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
			serverKeys.Delete(url)
			if logFile != nil {
				logFile.Close()
			}
		})
	}

	started := time.Now()
	for {
		// A look at the server's health follows each line that says it is
		// up, and a look every half second follows a server that says
		// nothing, which no llama-server we know is. Waiting the half
		// second before the first look was every stand-in server of the
		// tests waiting for it, #550.
		select {
		case <-ctx.Done():
			stop()
			return "", nil, ctx.Err()
		case <-exited:
			stop()
			// Killed while loading is what macOS does when memory runs
			// out, most often because a model is already loaded by
			// another llama-server.
			why := ""
			if strings.Contains(fmt.Sprint(exitErr), "killed") {
				why = " The system stopped it, which it does when memory runs out. Another " +
					"llama-server may be holding a model: quit the app, or find it with " +
					"pgrep -fl llama-server."
			}
			return "", nil, renderErr("%s stopped while loading the model (%v).%s Its output is "+
				"in %s", server, exitErr, why, filepath.Join(logDir, "llm-server.log"))
		case <-ready:
		case <-lookAgain(500 * time.Millisecond):
		}
		// How far the loading is, is the search's to say: it knows how long
		// it took before.
		if healthy(ctx, url) {
			took := time.Since(started).Seconds()
			// What loading costs is worth knowing in its parts, to know
			// which of them to make shorter.
			var parts []string
			if logFile != nil {
				for _, p := range loadPhases(logFile.Name(), took) {
					parts = append(parts, num.Fixed(p.seconds, 1)+"s "+p.name)
				}
			}
			if len(parts) > 0 {
				e.Log.Info("model loaded in %ss: %s", num.Fixed(took, 1), strings.Join(parts, ", "))
			} else {
				e.Log.Info("model loaded in %ss", num.Fixed(took, 1))
			}
			return url, stop, nil
		}
		if time.Since(started) > 10*time.Minute {
			stop()
			return "", nil, renderErr("the language model did not finish loading within 10 minutes")
		}
	}
}

// lookAgain is the wait before the next look at a loading server that has
// said nothing, a variable so a test can take it away and prove the
// server's own word is enough.
var lookAgain = time.After

// readyMarks are the lines llama-server writes once it may be ready: b11430
// says "model loaded" once the model is in memory and "server is listening"
// once it takes requests. Either is followed by a look at its health, which
// decides. An earlier line about its HTTP server listening while it still
// loads is looked at too, and found not ready yet.
var readyMarks = [][]byte{[]byte("model loaded"), []byte("server is listening")}

// hearReady passes llama-server's output on to out and says on ready
// whenever a line of it holds one of readyMarks. It keeps the end of what
// came before, so a mark split over two writes is heard too.
type hearReady struct {
	out   io.Writer
	ready chan<- struct{}
	tail  []byte
}

func (h *hearReady) Write(p []byte) (int, error) {
	// A log that cannot be written is no reason to stop hearing the
	// server, nor to stop the server, which would be sent a broken pipe.
	_, _ = h.out.Write(p)
	seen := append(h.tail, p...)
	// What is kept is the end a mark split over two writes may begin in,
	// and nothing of a mark already heard, so no mark is heard twice.
	from := len(seen)
	for _, mark := range readyMarks {
		from = min(from, len(seen)-len(mark)+1)
	}
	from = max(from, 0)
	for _, mark := range readyMarks {
		if at := bytes.LastIndex(seen, mark); at >= 0 {
			from = max(from, at+len(mark))
			select {
			case h.ready <- struct{}{}:
			default:
			}
		}
	}
	h.tail = append(h.tail[:0], seen[from:]...)
	return len(p), nil
}

// healthWait is how long one look at a loading server may take. A server
// that took the connection and never answers would otherwise hold the
// load past Cancel and past quitting, since the client has no timeout of
// its own: an answer from the model takes minutes.
var healthWait = 2 * time.Second

// healthy says whether the server at url says it is ready, asking for no
// longer than healthWait and no longer than ctx lasts.
func healthy(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, healthWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return false
	}
	authorize(request, url)
	response, err := localClient.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// loadPhase is a part of loading a model and how long it took.
type loadPhase struct {
	name    string
	seconds float64
}

// loadMarks are the lines llama-server b11430 writes as it begins each
// part of loading a model, at -lv 4, and what the part is called.
var loadMarks = []struct{ mark, name string }{
	{"loading model tensors", "reading the model"},
	{"constructing llama_context", "making room for the context"},
	{"warming up the model", "warming up"},
	{"model loaded", "getting ready to answer"},
}

// loadPhases reads from llama-server's log how long each part of loading
// took, the last ending when took seconds had gone by. Each line starts
// with the time since it started, minutes, seconds, milliseconds and
// microseconds, as in 0.19.335.706. A part whose line is not there is
// left out, and its time goes to the part before it.
func loadPhases(logPath string, took float64) []loadPhase {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	phases := []loadPhase{{"starting", 0}}
	began := 0.0
	next := 0
	for _, line := range strings.Split(string(data), "\n") {
		for i := next; i < len(loadMarks); i++ {
			if !strings.Contains(line, loadMarks[i].mark) {
				continue
			}
			at, ok := serverStamp(line)
			if !ok || at < began {
				break
			}
			phases[len(phases)-1].seconds = at - began
			phases = append(phases, loadPhase{loadMarks[i].name, 0})
			began, next = at, i+1
			break
		}
	}
	if len(phases) == 1 {
		return nil
	}
	phases[len(phases)-1].seconds = max(took-began, 0)
	return phases
}

func serverStamp(line string) (float64, bool) {
	words := runes.Fields(line)
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

// refusal is what llama-server said when it answered status with raw.
func refusal(status int, raw []byte) string {
	var refused chatError
	if json.Unmarshal(raw, &refused) == nil && refused.Error != nil {
		return refused.Error.Message
	}
	return fmt.Sprintf("status %d", status)
}

// CallLocal asks the model on this machine for a plan. The answer is read
// as it is written, and listen hears it arrive. What comes back is the
// answer with how the model got to it: how long it read and thought and
// wrote.
func (e *Engine) CallLocal(ctx context.Context, m LocalModel, r Recipe, prompt string,
	units, count, maxTokens int, logDir string, listen *Listener) (*localAnswer, error) {
	return e.askLocal(ctx, m, r, withSystem(r.System, chatMessage{"user", prompt}),
		units, count, maxTokens, logDir, "plan-prompt.txt", listen)
}

// CallLocalAgain asks once more in the same conversation: the request, the
// answer the model gave to it, and what is asked now. The start is the ask
// before word for word, so llama-server, still holding it, reads only the
// answer and the new question.
func (e *Engine) CallLocalAgain(ctx context.Context, m LocalModel, r Recipe, prompt, answer,
	again string, units, count, maxTokens int, logDir string, listen *Listener) (*localAnswer, error) {
	return e.askLocal(ctx, m, r, withSystem(r.System, chatMessage{"user", prompt},
		chatMessage{"assistant", answer}, chatMessage{"user", again}), units, count, maxTokens, logDir,
		"fit-prompt.txt", listen)
}

// noSystem is the system part of a recipe that has none, said to the API,
// which would otherwise take an empty one for the lines brief.
const noSystem = "\x00none"

// withSystem is a conversation with the recipe's system part first, when
// it has one. A recipe whose prompt is one message sends no system part.
func withSystem(system string, turns ...chatMessage) []chatMessage {
	if system == "" {
		return turns
	}
	return append([]chatMessage{{"system", system}}, turns...)
}

// chatMessage is one turn of a conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (e *Engine) askLocal(ctx context.Context, m LocalModel, r Recipe, messages []chatMessage,
	units, count, maxTokens int, logDir, promptFile string, listen *Listener) (*localAnswer, error) {
	// Nothing more of the transcript is read ahead of this search: the
	// server reads one thing at a time, and the asks after this one build
	// on what it leaves there, see readahead.go.
	e.ahead.close()
	url := strings.TrimRight(m.URL, "/")
	var stalled atomic.Bool
	if url == "" {
		// A model loaded while the transcript was still on its way is used
		// as it is. The search lets go of it when it is done, and it stops.
		chars := 0
		for _, message := range messages {
			if message.Role != "system" {
				chars += runes.Len(message.Content)
			}
		}
		size := localContextFor(m.Model, chars, maxTokens)
		if !modelReady(m.Model, size) {
			listen.part(partLoading)
		}
		held, release, err := e.holdModel(ctx, m, size, logDir)
		if err != nil {
			return nil, err
		}
		// A server that stopped answering is let go of at once, so it is
		// stopped and the next search loads one that works.
		defer func() {
			if stalled.Load() {
				release(0)
				return
			}
			release(m.Keep)
		}()
		url = held
	}
	listen.part(partReading)

	ask, maxTokens := localAsk(m, r, messages, units, count, maxTokens)
	body, err := json.Marshal(ask)
	if err != nil {
		return nil, err
	}
	if logDir != "" {
		turns := make([]string, len(messages))
		for i, message := range messages {
			turns[i] = "=== " + message.Role + " ===\n" + message.Content
		}
		path := filepath.Join(logDir, promptFile)
		if afero.WriteFile(e.Files, path, []byte(strings.Join(turns, "\n\n")), 0o644) == nil {
			listen.prompt(path)
		}
	}

	// The ask ends when the search does, and when the server has sent
	// nothing for localStall, see watchStall.
	asking, stop := context.WithCancel(ctx)
	defer stop()
	request, err := http.NewRequestWithContext(asking, http.MethodPost,
		url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("content-type", "application/json")
	authorize(request, url)
	gaveUp := func() error {
		return renderErr("the local model stopped answering: nothing came from it for %s seconds, "+
			"so it was stopped. Press Continue to start it afresh.", num.Fixed(localStall.Seconds(), 0))
	}
	watch := watchStall(localStall, func() {
		stalled.Store(true)
		e.Log.Warn("the local model sent nothing for %s seconds, so it is given up on",
			num.Fixed(localStall.Seconds(), 0))
		stop()
	})
	defer watch.Stop()

	started := time.Now()
	response, err := localClient.Do(request)
	if err != nil {
		if stalled.Load() {
			return nil, gaveUp()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, renderErr("the local model did not answer: %s", runes.Scrub(err.Error(), 200))
	}
	defer response.Body.Close()
	watch.Reset(localStall)
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		// A refusal is kept to read. An answer is kept in the saved reply,
		// with how the model got to it, so there is one file per answer.
		if logDir != "" {
			writeJSONText(e.Files, filepath.Join(logDir, "plan-error.json"), raw)
		}
		message := refusal(response.StatusCode, raw)
		// llama.cpp says only this when the graphics memory runs out partway,
		// and the usual reason is another program holding some of it.
		if strings.Contains(message, "Compute error") {
			return nil, renderErr("the local model ran out of memory while it worked, llama-server "+
				"says %q. Another program using the graphics memory, another language model above "+
				"all, is the usual cause. Close it and search again.", runes.Scrub(message, 100))
		}
		return nil, renderErr("the local model reported an error: %s", runes.Scrub(message, 400))
	}
	answer, err := readLocalStream(watch.reader(response.Body), listen)
	if err != nil {
		if stalled.Load() {
			return nil, gaveUp()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if runes.Trim(answer.Content) == "" {
		return nil, renderErr("the local model returned no answer")
	}
	reading, writing := 0.0, 0.0
	if t := answer.Timings; t != nil {
		reading, writing = t.PromptPerSecond, t.PredictedPerSecond
	}
	// A prompt that starts the way the one before did is read only from
	// where it differs, the transcript read ahead of a search above all,
	// see readahead.go, and the count of what was read says so, with how
	// long it took. All of it at that speed is what it would have taken.
	fresh := ""
	if t := answer.Timings; t != nil && t.PromptN > 0 && t.PromptN < answer.PromptTokens {
		fresh = fmt.Sprintf(", %s of them new in %ss", commas(t.PromptN), num.Fixed(t.PromptMS/1000, 1))
	}
	e.Log.Info("local model answered in %ss  read %s tok%s at %s tok/s  wrote %s tok at %s tok/s",
		num.Fixed(time.Since(started).Seconds(), 1),
		commas(answer.PromptTokens), fresh, num.Fixed(reading, 0),
		commas(answer.Written), num.Fixed(writing, 1))
	if answer.Reasoning > 0 {
		e.Log.Detail("the model thought for %s characters before it answered",
			commas(answer.Reasoning))
	}
	switch {
	case answer.FinishReason == "length" && r.Plain != nil:
		e.Log.Warn("the model went on writing after its answer and was stopped at %s tokens",
			commas(maxTokens))
	case answer.FinishReason == "length":
		e.Log.Warn("the answer hit the %s token ceiling. Raise --max-tokens if clips are missing.",
			commas(maxTokens))
	}
	return answer, nil
}

// localAsk is what llama-server is asked for these messages, and the most
// it may write. A search asks with it, and the transcript read ahead of a
// search is laid out by the server from the same, so it is laid out the
// way the search's is, see readahead.go.
func localAsk(m LocalModel, r Recipe, messages []chatMessage, units, count,
	maxTokens int) (map[string]any, int) {
	// A run works the budget out from its window before it asks, see
	// SuggestedThink. One that did not is given the budget for half an
	// hour rather than none at all.
	think := m.Think
	if think == ThinkForWindow {
		think = DefaultThink
	}
	// A plain answer is a few tokens a clip, so it may run little past its
	// thinking, see answerCap. A thought without a budget is not capped.
	if r.Plain != nil && think >= 0 {
		maxTokens = min(maxTokens, r.Plain.answerCap(think, count))
	}
	ask := map[string]any{
		"model":      filepath.Base(m.Model),
		"messages":   messages,
		"max_tokens": maxTokens,
		// How long the model may think. The server stops the thought at the
		// budget and closes it with the message, so the answer follows.
		"reasoning_budget_tokens":  max(think, -1),
		"reasoning_budget_message": thinkEnough,
		// Streamed, with the reading of the prompt reported as it goes and
		// what it cost at the end, which a stream otherwise leaves out.
		"stream":          true,
		"stream_options":  map[string]any{"include_usage": true},
		"return_progress": true,
	}
	// An answer without JSON is held to nothing. A grammar of our own
	// would hold the model from its first token, so Gemma could not open
	// its thought, which only a schema's grammar leaves room for, and it
	// wrote its thinking into an answer line that never ended. The reader
	// passes over any line that is not a clip.
	if r.Plain == nil {
		schema := json.RawMessage(r.Schema(units, count))
		ask["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "plan", "strict": true, "schema": schema,
			},
		}
	}
	if m.Seed != 0 {
		ask["seed"] = m.Seed
	}
	if m.Temperature != nil {
		ask["temperature"] = *m.Temperature
	}
	return ask, maxTokens
}
