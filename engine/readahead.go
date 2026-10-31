package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"framefairy/engine/num"
	"framefairy/engine/runes"
	"framefairy/engine/timed"
)

// ---------------------------------------------------------------------------
// The transcript read ahead of a search
//
// A local model reads the whole prompt before it thinks a word, about 30
// seconds for a half hour window on an M2 Max, and the prompt is mostly
// the transcript. The first search of a new video is asked for while its
// transcript is still being heard. So while the transcript grows, the
// model is given what of it is settled to read, and llama-server keeps
// what it read for the next ask, -np 1 in local.go. The search then finds
// most of its prompt read and reads only the rest.
//
// What is read has to be the start of what the search asks, token for
// token. llama-server keeps what it read only as far as the next ask
// starts the same, and a model that looks back over a window, as Gemma 4
// does, cannot go back to an earlier point of what it read without a copy
// of the window kept from there, a checkpoint. So the transcript is
// written out by the search's own request, searchRequest, from the lines
// heard so far but the last, which may still grow. The server lays the
// request out with the model's own template and splits it into tokens,
// and of those only the ones that stay whatever follows are read: the
// tokens the request with one line more starts with too. They are read
// with a completion that writes nothing, so the server holds exactly
// them, and the search's ask, laid out and split the same way, starts
// with them.
//
// Reading ahead stops before the search asks. The server reads one thing
// at a time, so a read still under way when the search asks is called
// off, and what it read so far is kept. A read after the search asked
// would take the server back to the start of the search's prompt, and
// the question that follows the answer in the same conversation, see
// fit.go, would read it all again.
//
// What a search asks after the transcript, how many clips and which lines
// are taken, is not read ahead. It is a few lines, and it is only known
// once the search starts.
// ---------------------------------------------------------------------------

// ReadAhead is what of a search's transcript the local model read before
// the search asked.
type ReadAhead struct {
	// Reads is how many times the model was given more to read.
	Reads int
	// Tokens is how many tokens of the prompt it held after the last
	// read, and Read how many it read in all, more than Tokens when what
	// was heard later wrote an earlier line out differently.
	Tokens, Read int
	// Rewritten is how many reads found a line read before written out
	// differently. It should stay 0: the loudness a pause is told from is
	// fixed by the first part heard, see floorSpan, so a part heard later
	// moves no earlier word, issue #437. A read that counts one says that
	// something else writes heard lines out again.
	Rewritten int
	// Seconds is how long the reading took, in all.
	Seconds float64
}

// aheadGate lets the transcript be read ahead of a search until the
// search asks, and never after. An engine has one, for the one search it
// runs, see Engine.ahead.
type aheadGate struct {
	mu     sync.Mutex
	closed bool
	// over is closed with the gate, so a reader waiting for the
	// transcript to grow stops waiting.
	over chan struct{}
	// stop calls off the read under way, and done is closed when it has
	// ended.
	stop context.CancelFunc
	done chan struct{}
}

// start begins a read, with a context that ends when ctx does or when the
// search asks, and the end that has to be called when the read is over.
// It is false once the search has asked.
func (g *aheadGate) start(ctx context.Context) (context.Context, func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, nil, false
	}
	reading, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	g.stop, g.done = stop, done
	return reading, func() {
		stop()
		g.mu.Lock()
		g.stop, g.done = nil, nil
		g.mu.Unlock()
		close(done)
	}, true
}

// ended is closed once the gate is.
func (g *aheadGate) ended() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.over == nil {
		g.over = make(chan struct{})
	}
	return g.over
}

// close calls off a read under way, waits until it has ended, and lets no
// read start after it. The search closes it when it asks, and when it
// ends without asking.
func (g *aheadGate) close() {
	g.mu.Lock()
	if !g.closed {
		g.closed = true
		if g.over == nil {
			g.over = make(chan struct{})
		}
		close(g.over)
	}
	stop, done := g.stop, g.done
	g.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}

// readAhead has local, loaded with size, read the transcript of the
// window of req as it is heard, until all of the window is heard, the
// search asks or ends, or ctx ends. The model is held only while it
// reads, and only when nobody else holds it, so a search of another video
// has it whenever it asks.
func (p *Project) readAhead(ctx context.Context, req PlanRequest, local LocalModel, size int) ReadAhead {
	var did ReadAhead
	path := filepath.Join(p.LogsDir(), TranscriptName(nil))
	var last aheadRead
	over := p.engine.ahead.ended()
	for {
		grew := transcriptGrows(path)
		// No transcript yet is a transcript to wait for.
		heard, ok := p.heardSoFar(req)
		if ok && heard.whole {
			return did
		}
		// The last line may still grow, so it is not read. Two lines are
		// the least that leave something.
		if ok && len(heard.lines) >= 2 {
			now, err := p.readHeard(ctx, heard, local, size, last)
			switch {
			case errors.Is(err, errAheadDone):
				return did
			case err != nil:
				if ctx.Err() == nil {
					p.engine.Log.Detail("the transcript is no longer read ahead of the search: %s", err)
				}
				return did
			case now.lost:
				// Someone else had the server, which holds their prompt now.
				last = aheadRead{}
			case now.tokens != nil:
				did.Reads++
				did.Tokens, did.Read = len(now.tokens), did.Read+now.read
				did.Seconds += now.took.Seconds()
				if now.rewritten {
					did.Rewritten++
				}
				last = now
			}
		}
		select {
		case <-grew:
		case <-over:
			return did
		case <-ctx.Done():
			return did
		}
	}
}

// errAheadDone ends reading ahead without anything having gone wrong: the
// search asked, or what it asks cannot be read ahead.
var errAheadDone = errors.New("reading ahead is over")

// heardWindow is what of a search's window is heard so far.
type heardWindow struct {
	opts Options
	// lines are the lines heard without a gap from the start of the
	// window, and whole says whether that is all of it.
	lines []Line
	whole bool
}

// heardSoFar is the window of req as far as it is heard, with the options
// its search asks with. The window and the options are worked out the way
// the search works them out, see Plan and prepare, so its lines are the
// search's lines.
func (p *Project) heardSoFar(req PlanRequest) (heardWindow, bool) {
	file, _, _, ok := readEpisodeTranscript(p.Files, p.Source, p.Base.ASRModel)
	duration := float64(file.End)
	if !ok || duration <= 0 {
		return heardWindow{}, false
	}
	window := inMilliseconds(searchWindow(req.From, req.To, req.Pass, duration))
	span := timed.Window{Start: 0, End: duration}
	if window != nil {
		span = *window
	}
	opts, _ := p.planOptions(req, window)
	opts = opts.forSpan(span)
	// Heard up to the first gap, by the same rule the search hears its
	// window by, which leaves gaps under a tenth of a second alone.
	gaps := gapsIn(file.heard(), span)
	heard := heardWindow{opts: opts, whole: len(gaps) == 0}
	if heard.whole || gaps[0].Start <= span.Start {
		return heard, true
	}
	stamp, err := stampOf(p.Files, p.Source)
	if err != nil {
		return heardWindow{}, false
	}
	t, ok := sliceTranscript(p.Files, filepath.Join(p.LogsDir(), TranscriptName(nil)), stamp,
		filepath.Base(filepath.Clean(speechModel(opts))), timed.Window{Start: span.Start, End: gaps[0].Start}, opts.SilenceDB)
	if !ok {
		return heardWindow{}, false
	}
	heard.lines = searchLines(t, opts)
	return heard, true
}

// aheadRead is one read: the server it was read into, the tokens it
// holds after it, how many of them it read, and how long that took. No
// tokens is a read that had nothing new, or none at all, because the
// model was not there to read or someone else took it, which lost says.
// rewritten says that a line read before was written out differently.
type aheadRead struct {
	url       string
	tokens    []int
	read      int
	took      time.Duration
	lost      bool
	rewritten bool
}

// readHeard has the model read what of heard is settled, unless it holds
// that already. last is the read before. It reads only into the model
// loaded ahead of the search, while nobody holds it, and gives way to any
// search that asks for it while it reads, see holdIdle.
func (p *Project) readHeard(ctx context.Context, heard heardWindow, local LocalModel, size int,
	last aheadRead) (did aheadRead, err error) {
	asked := askedWith(heard.opts)
	if !readsAhead(heard.lines, asked) {
		p.engine.Log.Detail("the %s recipe asks something before the transcript, so the transcript is "+
			"not read ahead of the search", asked.recipe().Name)
		return aheadRead{}, errAheadDone
	}
	reading, end, ok := p.engine.ahead.start(ctx)
	if !ok {
		return aheadRead{}, errAheadDone
	}
	defer end()
	// The model given way to is a read lost, not a fault.
	reading, giveWay := context.WithCancelCause(reading)
	defer giveWay(nil)
	defer func() {
		switch {
		case err == nil || ctx.Err() != nil:
		case errors.Is(context.Cause(reading), errModelBusy):
			did, err = aheadRead{lost: true}, nil
		case reading.Err() != nil:
			// Called off because the search asked.
			err = errAheadDone
		}
	}()
	url, release, ok := holdIdle(local.Model, size, giveWay)
	if !ok {
		return aheadRead{lost: true}, nil
	}
	defer release(warmKeep)
	held := last.tokens
	if last.url != url {
		held = nil
	}
	settled, err := settledTokens(reading, url, local, asked, heard.lines)
	if err != nil {
		return aheadRead{}, err
	}
	same := commonPrefix(settled, held)
	if same == len(settled) {
		return aheadRead{}, nil
	}
	rewritten := same < len(held)
	if rewritten {
		p.engine.Log.Detail("the transcript heard so far is written out differently from token %s of %s on, "+
			"so the model reads it again from there, or from the start where it kept no copy of what it "+
			"read before that token", num.Thousands(same), num.Thousands(len(held)))
	}
	started := time.Now()
	read, err := readTokens(reading, url, settled)
	if err != nil {
		return aheadRead{}, err
	}
	took := time.Since(started)
	p.engine.Log.Detail("the model read %s tokens of the transcript ahead of the search in %ss, and holds %s",
		num.Thousands(read), num.Fixed(took.Seconds(), 1), num.Thousands(len(settled)))
	return aheadRead{url: url, tokens: settled, read: read, took: took, rewritten: rewritten}, nil
}

// readsAhead says whether a search asked with opts starts its request with
// the transcript, so what is read ahead is the start of what it asks. A
// recipe that says first how many lines there are, or that writes a line
// out by how it compares with all the others, changes its request from
// the start with every line heard.
func readsAhead(lines []Line, opts PlanOptions) bool {
	recipe, _, prompt := searchRequest(lines, opts)
	if recipe.Transcript == nil {
		return false
	}
	return strings.HasPrefix(prompt, recipe.Transcript(lines, opts))
}

// settledTokens are the tokens of the search's prompt for lines that stay
// the same however the transcript goes on: what the prompt for all lines
// but the last and the prompt for all of them start with, as the server
// lays them out and splits them, see the top of this file.
func settledTokens(ctx context.Context, url string, m LocalModel, asked PlanOptions,
	lines []Line) ([]int, error) {
	var settled []int
	for i, n := range []int{len(lines) - 1, len(lines)} {
		recipe, units, prompt := searchRequest(lines[:n], asked)
		ask, _ := localAsk(m, recipe, withSystem(recipe.System, chatMessage{"user", prompt}),
			len(units), asked.Count, asked.MaxTokens)
		var laid struct {
			Prompt string `json:"prompt"`
		}
		if err := serverCall(ctx, url, "/apply-template", ask, &laid, localStall); err != nil {
			return nil, err
		}
		var split struct {
			Tokens []int `json:"tokens"`
		}
		// Split as the server splits a prompt it is asked with, special
		// tokens added and read, see tokenize_input_prompts in llama.cpp.
		if err := serverCall(ctx, url, "/tokenize", map[string]any{"content": laid.Prompt,
			"add_special": true, "parse_special": true}, &split, localStall); err != nil {
			return nil, err
		}
		if i == 0 {
			settled = split.Tokens
			continue
		}
		settled = settled[:commonPrefix(settled, split.Tokens)]
	}
	return settled, nil
}

// readTokens has the server read tokens and keep them, writing nothing,
// and says how many it read rather than found kept.
func readTokens(ctx context.Context, url string, tokens []int) (int, error) {
	var answer struct {
		Timings *localTimings `json:"timings"`
	}
	// A slow machine reads a few tokens a second, see slowestReading, and
	// says nothing while it does: the answer comes when it is done. What
	// the server kept is its own to know, so the time is for all of them.
	limit := localStall + time.Duration(float64(len(tokens))/slowestReading*float64(time.Second))
	err := serverCall(ctx, url, "/completion", map[string]any{
		"prompt": tokens, "n_predict": 0, "cache_prompt": true,
		// Only what it cost, not the prompt back.
		"response_fields": []string{"timings"},
	}, &answer, limit)
	if err != nil {
		return 0, err
	}
	if answer.Timings == nil {
		return len(tokens), nil
	}
	return answer.Timings.PromptN, nil
}

// serverCall posts ask to llama-server at url and reads what it answers
// into answer, giving up after limit. It is for the calls answered in one
// piece. A search's ask is answered as it is written, and askLocal reads
// it so, with its own watch on a server that falls silent.
func serverCall(ctx context.Context, url, path string, ask, answer any, limit time.Duration) error {
	body, err := json.Marshal(ask)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")
	authorize(request, url)
	response, err := localClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", path, runes.Scrub(refusal(response.StatusCode, raw), 200))
	}
	return json.Unmarshal(raw, answer)
}

// commonPrefix is how many tokens a and b start with alike.
func commonPrefix(a, b []int) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
