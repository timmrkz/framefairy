package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// APIURL is the only endpoint this tool contacts. It is hardcoded and never
// derived from any input.
const APIURL = "https://api.anthropic.com/v1/messages"

const apiVersion = "2023-06-01"

// DefaultModel plans an episode unless told otherwise.
const DefaultModel = "claude-sonnet-5"

// ModelFacts are prices in US dollars per million tokens, the context window
// and the maximum output. From the models overview in Anthropic's docs and
// the pricing page linked from it. These are the kind of numbers that go
// stale, so check them if a run's estimate looks wrong.
type ModelFacts struct {
	PriceIn   float64
	PriceOut  float64
	Priced    bool
	Context   int
	MaxOutput int
}

var models = map[string]ModelFacts{
	"claude-fable-5-1":          {10.0, 50.0, true, 1_000_000, 128_000},
	"claude-opus-5":             {5.0, 25.0, true, 1_000_000, 128_000},
	"claude-sonnet-5":           {2.0, 10.0, true, 1_000_000, 128_000},
	"claude-haiku-4-5-20251001": {1.0, 5.0, true, 200_000, 64_000},
}

// FactsFor gives prices and limits for a model, or conservative defaults for
// one this table has not heard of. An unknown model gets no cost estimate
// rather than a made-up one.
func FactsFor(model string) ModelFacts {
	if facts, ok := models[model]; ok {
		return facts
	}
	return ModelFacts{Context: 200_000, MaxOutput: 64_000}
}

// HMS formats seconds as h:mm:ss.
func HMS(seconds float64) string {
	total := int(max(0, seconds))
	return fmt.Sprintf("%d:%02d:%02d", total/3600, total%3600/60, total%60)
}

func money(model string, tokensIn, tokensOut int) string {
	facts := FactsFor(model)
	if !facts.Priced {
		return "cost unknown for this model"
	}
	return "approx $" + fixed(float64(float64(tokensIn)/1e6*facts.PriceIn)+
		float64(float64(tokensOut)/1e6*facts.PriceOut), 3)
}

// Transient codes are the API asking to be asked again later.
var transientCodes = map[int]bool{408: true, 409: true, 425: true, 429: true,
	500: true, 502: true, 503: true, 504: true, 529: true}

const prefillRejected = "does not support assistant message prefill"

const maxAttempts = 4

const repairSystem = "You repair malformed JSON. The user sends text that was meant to be a " +
	"single JSON object. Return only the corrected JSON object, nothing else. " +
	"Do not invent data: keep every value that is present and drop anything " +
	"incomplete."

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func messages(prompt string, prefill bool) []message {
	out := []message{{"user", prompt}}
	if prefill {
		out = append(out, message{"assistant", "{"})
	}
	return out
}

// keyIsSane rejects a key that cannot go in an HTTP header. A stray newline
// in a stored key is the usual culprit.
func keyIsSane(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if r == '\r' || r == '\n' || r == 0 || r > 127 {
			return false
		}
	}
	return true
}

// ReadAPIKey takes the key from the environment first, then the macOS
// keychain. Never from a file on disk.
func ReadAPIKey(ctx context.Context) (string, error) {
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	source := "ANTHROPIC_API_KEY"
	if key == "" {
		res := run(ctx, "", "security", "find-generic-password",
			"-a", "framefairy", "-s", "anthropic-api-key", "-w")
		if res.Code == 0 && strip(res.Stdout) != "" {
			key, source = strip(res.Stdout), "the keychain"
		}
	}
	if key == "" {
		return "", renderErr("no API key found. Either set ANTHROPIC_API_KEY, or store it " +
			"in the keychain with:\n" +
			"  security add-generic-password -a framefairy -s anthropic-api-key -w")
	}
	if !keyIsSane(key) {
		return "", renderErr("the API key from %s contains characters that cannot go in "+
			"an HTTP header. It was probably stored with a stray newline.", source)
	}
	return key, nil
}

// StoreAPIKey puts a key in the macOS keychain, which is the only place the
// app keeps one. Never a file on disk, and never the settings, which are
// plain JSON in the config folder and get copied about.
//
// An empty key removes the stored one. That is how somebody takes their key
// off a machine, so it has to be an ordinary thing to do rather than an
// error.
//
// The key is checked before it is stored rather than only when it is used.
// A key pasted with a newline on the end is the common case, and finding
// that out at the moment it is typed beats finding out when an episode has
// already been transcribed.
func StoreAPIKey(key string) error {
	key = strings.TrimSpace(key)
	if runtime.GOOS != "darwin" {
		return renderErr("this machine has no keychain to put a key in. " +
			"Set ANTHROPIC_API_KEY instead.")
	}
	ctx := context.Background()
	if key == "" {
		// Nothing stored is not a failure, so the result is not looked at.
		run(ctx, "", "security", "delete-generic-password",
			"-a", "framefairy", "-s", "anthropic-api-key")
		return nil
	}
	if !keyIsSane(key) {
		return renderErr("that key has characters in it that cannot go in an HTTP header.")
	}
	res := run(ctx, "", "security", "add-generic-password",
		"-U", "-a", "framefairy", "-s", "anthropic-api-key", "-w", key)
	if res.Code != 0 {
		return renderErr("the keychain refused the key: %s", strip(res.Stderr))
	}
	return nil
}

// writeJSONText writes a JSON string indented, so a log file can be read by
// a person.
func writeJSONText(path string, body []byte) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err == nil {
		body = pretty.Bytes()
	}
	_ = os.WriteFile(path, body, 0o644)
}

// httpClient never follows redirects. A redirect would resend the key header
// to whatever host it names, and nothing legitimate needs one.
var httpClient = &http.Client{
	Timeout: 10 * time.Minute,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type apiReply struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens          int `json:"input_tokens"`
		OutputTokens         int `json:"output_tokens"`
		CacheReadInputTokens int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// post sends one request, retried on the errors that are worth retrying.
//
// A 429 or a 529 is the API asking to be asked again later. Failing on those
// throws away the input tokens already paid for, which is the expensive way
// to handle a temporary condition.
func (e *Engine) post(ctx context.Context, build func(prefill bool) ([]byte, error),
	logDir, tag string) (*apiReply, []byte, error) {
	delay := 2.0
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		e.Calls.Attempts++
		if attempt > 1 {
			e.Log.Info("API attempt %d of %d", attempt, maxAttempts)
		}
		payload, err := build(e.Prefill)
		if err != nil {
			return nil, nil, err
		}
		key, err := ReadAPIKey(ctx)
		if err != nil {
			return nil, nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, APIURL,
			bytes.NewReader(payload))
		if err != nil {
			return nil, nil, err
		}
		request.Header.Set("content-type", "application/json")
		request.Header.Set("anthropic-version", apiVersion)
		request.Header.Set("x-api-key", key)

		response, err := httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if attempt < maxAttempts {
					e.Log.Warn("the API did not respond in time, retrying (attempt %d of %d)",
						attempt, maxAttempts)
					continue
				}
				return nil, nil, renderErr("the API did not respond within 10 minutes")
			}
			if attempt < maxAttempts {
				e.Log.Warn("network error (%s), retrying in %ss", Scrub(err.Error(), 80),
					fixed(delay, 0))
				if err := sleepCtx(ctx, time.Duration(delay*float64(time.Second))); err != nil {
					return nil, nil, err
				}
				delay = min(delay*2, 60)
				continue
			}
			return nil, nil, renderErr("cannot reach the API: %s", Scrub(err.Error(), 200))
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if attempt < maxAttempts {
				e.Log.Warn("network error (%s), retrying in %ss", Scrub(readErr.Error(), 80),
					fixed(delay, 0))
				if err := sleepCtx(ctx, time.Duration(delay*float64(time.Second))); err != nil {
					return nil, nil, err
				}
				delay = min(delay*2, 60)
				continue
			}
			return nil, nil, renderErr("cannot reach the API: %s", Scrub(readErr.Error(), 200))
		}

		if response.StatusCode < 200 || response.StatusCode > 299 {
			detail := strings.ToValidUTF8(string(body), "\uFFFD")
			if logDir != "" {
				writeJSONText(filepath.Join(logDir, fmt.Sprintf("%s-error-%d.json", tag, attempt)), body)
			}
			if strings.Contains(detail, prefillRejected) && e.Prefill {
				e.Prefill = false
				e.Log.Warn("this model does not accept a prefilled reply, sending the " +
					"request without one. Drop --prefill to skip this round trip next time.")
				continue
			}
			code := response.StatusCode
			if transientCodes[code] && attempt < maxAttempts {
				wait := delay
				header := strings.TrimSpace(response.Header.Get("retry-after"))
				if n, err := strconv.Atoi(header); err == nil && header != "" &&
					!strings.ContainsAny(header, "+-") {
					wait = min(float64(n), 60)
				}
				e.Log.Warn("API returned %d, retrying in %ss (attempt %d of %d)",
					code, fixed(wait, 0), attempt, maxAttempts)
				if err := sleepCtx(ctx, time.Duration(wait*float64(time.Second))); err != nil {
					return nil, nil, err
				}
				delay = min(delay*2, 60)
				continue
			}
			hint := ""
			switch code {
			case 401:
				hint = " The key was rejected. Check it in the Console."
			case 400:
				hint = " The request itself was refused, so this will not succeed on a retry."
			}
			return nil, nil, renderErr("the API returned %d: %s%s", code, Scrub(detail, 400), hint)
		}

		if logDir != "" {
			writeJSONText(filepath.Join(logDir, tag+"-response.json"), body)
		}
		if attempt > 1 {
			e.Log.OK("the API answered on attempt %d", attempt)
		}
		var data apiReply
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, nil, renderErr("the API returned a body that is not JSON")
		}
		if data.Type == "error" {
			message := data.Error.Message
			if message == "" {
				message = "unknown"
			}
			return nil, nil, renderErr("the API reported an error: %s", Scrub(message, 400))
		}
		return &data, body, nil
	}
	return nil, nil, renderErr("the API could not be reached after several attempts")
}

// Starting points for the estimate, replaced by measured values as soon as
// this tool has made a call. 1.9 characters per token matches German
// transcripts, where the English rule of thumb of 4 is nowhere near. The
// output figure is large because current models think before answering and
// those tokens are billed as output. On a planning call they are most of the
// bill.
const (
	charsPerToken = 1.9
	assumedOutput = 14000
)

type usageEntry struct {
	Tag    *string  `json:"tag,omitempty"`
	Chars  *float64 `json:"chars,omitempty"`
	Input  *float64 `json:"input,omitempty"`
	Output *float64 `json:"output,omitempty"`
}

type usageRecord struct {
	Tag    string `json:"tag"`
	Chars  int    `json:"chars"`
	Input  int    `json:"input"`
	Output int    `json:"output"`
}

func loadUsage(logDir string) ([]json.RawMessage, bool) {
	data, err := os.ReadFile(filepath.Join(logDir, "usage.json"))
	if err != nil {
		return nil, false
	}
	var history []json.RawMessage
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, false
	}
	return history, true
}

// recordUsage remembers what a call actually cost, so the next estimate is
// not a guess. Entries are tagged by which call it was, because planning and
// other calls can have output sizes an order of magnitude apart.
func recordUsage(logDir string, chars int, data *apiReply, tag string) {
	if logDir == "" || data.Usage.InputTokens <= 0 {
		return
	}
	history, _ := loadUsage(logDir)
	entry, _ := json.Marshal(usageRecord{tag, chars, data.Usage.InputTokens, data.Usage.OutputTokens})
	history = append(history, entry)
	if len(history) > 40 {
		history = history[len(history)-40:]
	}
	body, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(logDir, 0o755)
	_ = os.WriteFile(filepath.Join(logDir, "usage.json"), body, 0o644)
}

// estimateUsage gives expected input and output tokens for this kind of call,
// measured where possible. Only past calls of the same kind count.
func estimateUsage(logDir string, chars int, tag string) (int, int, bool) {
	var ratios, outputs []float64
	count := 0
	if logDir != "" {
		history, _ := loadUsage(logDir)
		for _, raw := range history {
			var h usageEntry
			if err := json.Unmarshal(raw, &h); err != nil {
				continue
			}
			if h.Input == nil || *h.Input == 0 {
				continue
			}
			entryTag := "plan"
			if h.Tag != nil {
				entryTag = *h.Tag
			}
			if entryTag != tag {
				continue
			}
			count++
			if h.Chars != nil && *h.Chars != 0 {
				ratios = append(ratios, *h.Chars / *h.Input)
			}
			if h.Output != nil {
				outputs = append(outputs, *h.Output)
			}
		}
	}
	if count > 0 {
		sort.Float64s(ratios)
		sort.Float64s(outputs)
		ratio := charsPerToken
		if len(ratios) > 0 {
			ratio = ratios[len(ratios)/2]
		}
		output := float64(assumedOutput)
		if len(outputs) > 0 {
			output = outputs[len(outputs)/2]
		}
		return int(float64(chars) / max(ratio, 0.5)), int(output), true
	}
	return int(float64(chars) / charsPerToken), assumedOutput, false
}

func (e *Engine) reportUsage(data *apiReply, seconds float64, model string) {
	in, out, cached := data.Usage.InputTokens, data.Usage.OutputTokens,
		data.Usage.CacheReadInputTokens
	if facts := FactsFor(model); facts.Priced {
		e.Calls.Spent += float64(float64(in)/1e6*facts.PriceIn) +
			float64(float64(out)/1e6*facts.PriceOut)
	}
	cachedNote := ""
	if cached != 0 {
		cachedNote = fmt.Sprintf(" (%s cached)", commas(cached))
	}
	stop := data.StopReason
	if stop == "" {
		stop = "?"
	}
	e.Log.Info("model replied in %ss  in %s tok%s  out %s tok  stop=%s  %s",
		fixed(seconds, 1), commas(in), cachedNote, commas(out), stop, money(model, in, out))
}

func replyText(data *apiReply) string {
	var b strings.Builder
	for _, block := range data.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

type requestBody struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

// CallClaude makes one call. The reply is on disk before anything parses it.
func (e *Engine) CallClaude(ctx context.Context, prompt, model string, maxTokens int,
	logDir, tag, system string) (string, error) {
	e.Calls.Requests++
	if system == "" {
		system = SystemPrompt
	}
	build := func(prefill bool) ([]byte, error) {
		return json.Marshal(requestBody{model, maxTokens, system, messages(prompt, prefill)})
	}
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return "", err
		}
		// The whole request, not half of it, so the log shows what the model
		// was actually asked.
		_ = os.WriteFile(filepath.Join(logDir, tag+"-prompt.txt"),
			[]byte("=== system ===\n"+system+"\n\n=== user ===\n"+prompt), 0o644)
	}
	e.Log.Detail("POST %s model=%s max_tokens=%d prompt=%s chars", APIURL, model,
		maxTokens, commas(runeLen(prompt)))
	started := time.Now()
	data, _, err := e.post(ctx, build, logDir, tag)
	if err != nil {
		return "", err
	}
	e.reportUsage(data, time.Since(started).Seconds(), model)
	recordUsage(logDir, runeLen(prompt), data, tag)

	text := replyText(data)
	if strip(text) == "" {
		kindSet := map[string]bool{}
		for _, block := range data.Content {
			kind := block.Type
			if kind == "" {
				kind = "?"
			}
			kindSet[kind] = true
		}
		var kinds []string
		for k := range kindSet {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		kindText := "none"
		if len(kinds) > 0 {
			kindText = pyListRepr(kinds)
		}
		stop := data.StopReason
		if stop == "" {
			stop = "?"
		}
		return "", renderErr("the reply contained no text (blocks: %s, stop=%s)", kindText, stop)
	}
	if data.StopReason == "max_tokens" {
		e.Log.Warn("the reply hit the %s token ceiling and was cut off. Trying to "+
			"salvage the complete clips from it. Raise --max-tokens or lower --count "+
			"if that is not enough.", commas(maxTokens))
	}
	if e.Prefill {
		return "{" + text, nil
	}
	return text, nil
}

// CallClaudeWithHeadroom asks, and if the model spent the whole ceiling
// thinking, gives it more.
//
// A reply that is entirely thinking and no answer is not a failure of the
// request, it is a ceiling set too low for how hard the model found the task.
// Failing there throws away everything already paid for, so the ceiling is
// raised once and the question asked again.
func (e *Engine) CallClaudeWithHeadroom(ctx context.Context, prompt, model string,
	maxTokens int, logDir, tag string) (string, error) {
	facts := FactsFor(model)
	text, err := e.CallClaude(ctx, prompt, model, maxTokens, logDir, tag, "")
	if err == nil {
		return text, nil
	}
	spentThinking := strings.Contains(err.Error(), "blocks: ['thinking']")
	headroom := min(maxTokens*2, facts.MaxOutput)
	if !spentThinking || headroom <= maxTokens {
		return "", err
	}
	e.Log.Warn("the model used the whole %s token ceiling thinking and never "+
		"answered. Asking again with %s.", commas(maxTokens), commas(headroom))
	return e.CallClaude(ctx, prompt, model, headroom, logDir, tag, "")
}

// RepairJSON asks the model to fix its own output, sending only the broken
// text. Resending the transcript to fix a formatting problem means paying for
// the whole episode a second time, and the transcript was never the part that
// was wrong.
func (e *Engine) RepairJSON(ctx context.Context, broken, model, logDir string) (string, error) {
	build := func(prefill bool) ([]byte, error) {
		return json.Marshal(requestBody{model, min(8000, runeLen(broken)/2+2000),
			repairSystem, messages(Scrub(broken, 60000), prefill)})
	}
	e.Calls.Requests++
	e.Log.Info("asking for a repair of %s characters (the transcript is not resent)",
		commas(runeLen(broken)))
	started := time.Now()
	data, _, err := e.post(ctx, build, logDir, "repair")
	if err != nil {
		return "", err
	}
	e.reportUsage(data, time.Since(started).Seconds(), model)
	text := replyText(data)
	if e.Prefill {
		return "{" + text, nil
	}
	return text, nil
}
