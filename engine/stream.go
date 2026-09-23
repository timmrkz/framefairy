package engine

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// ---------------------------------------------------------------------------
// Answers as they are written
//
// A model writes its answer a few characters at a time, and both ways of
// asking one, the API and llama-server on this machine, can hand those
// characters over as they come. Waiting for the whole answer is waiting for
// the last clip before the first can be shown, and a search asks for twelve.
// So the answer is read as a stream, and whoever is listening hears it
// arrive.
// ---------------------------------------------------------------------------

// Listener hears an answer while it is being written. Every field may be
// nil.
type Listener struct {
	// Text is the answer itself, a piece at a time, in order.
	Text func(piece string)
	// Thinking is the model reasoning before it answers. Only how much of
	// it there is, because nobody reads it and the API does not show it.
	Thinking func(chars int)
	// Reading is the model working through the prompt before it writes a
	// word, in tokens read out of tokens to read. Only llama-server says.
	Reading func(done, total int)
	// Part is the call moving on to the next part of its work: loading the
	// model, then reading the prompt. Writing begins with the first text.
	Part func(name string)
}

func (l *Listener) part(name string) {
	if l != nil && l.Part != nil {
		l.Part(name)
	}
}

func (l *Listener) text(piece string) {
	if l != nil && l.Text != nil && piece != "" {
		l.Text(piece)
	}
}

func (l *Listener) thinking(chars int) {
	if l != nil && l.Thinking != nil && chars > 0 {
		l.Thinking(chars)
	}
}

func (l *Listener) reading(done, total int) {
	if l != nil && l.Reading != nil && total > 0 {
		l.Reading(done, total)
	}
}

// sseLines reads server-sent events: an event name, then data, then a blank
// line. Each event is handed over with its name, which the API sends and
// llama-server does not.
func sseLines(r io.Reader, each func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	// A line is one event's data. They are small, but the first one the API
	// sends describes the whole message, so there is room for more.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	event := ""
	var data []string
	flush := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		joined := strings.Join(data, "\n")
		name := event
		event, data = "", data[:0]
		return each(name, joined)
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// A comment, which is how a server keeps a quiet line open.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

// streamError is a stream that ended in an error the server sent rather
// than one the network made. Its kind says whether it is worth asking again.
type streamError struct {
	kind    string
	message string
}

func (e *streamError) Error() string {
	if e.kind == "" {
		return e.message
	}
	return e.kind + ": " + e.message
}

// transient says whether the API would answer if asked again later.
func (e *streamError) transient() bool {
	switch e.kind {
	case "overloaded_error", "api_error", "rate_limit_error", "timeout_error":
		return true
	}
	return false
}

// readClaudeStream reads the API's answer as it is written and puts it back
// together as the reply a plain request would have had, so everything after
// it reads the same whichever way it was asked.
//
// It says whether any of the answer was handed on. An answer that broke off
// before a word of it was heard can simply be asked for again. One that
// broke off after has already been acted on.
func readClaudeStream(r io.Reader, listen *Listener) (*apiReply, bool, error) {
	var reply apiReply
	reply.Type = "message"
	heard := false
	done := false
	err := sseLines(r, func(event, data string) error {
		var frame struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				Usage struct {
					InputTokens          int `json:"input_tokens"`
					CacheReadInputTokens int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				Thinking   string `json:"thinking"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			return renderErr("the API sent a piece of its answer that is not JSON")
		}
		kind := frame.Type
		if kind == "" {
			kind = event
		}
		switch kind {
		case "message_start":
			reply.Usage.InputTokens = frame.Message.Usage.InputTokens
			reply.Usage.CacheReadInputTokens = frame.Message.Usage.CacheReadInputTokens
		case "content_block_start":
			// Blocks arrive in order and are numbered from nought. One out
			// of order is a stream this does not understand.
			if frame.Index != len(reply.Content) {
				return renderErr("the API's answer arrived out of order")
			}
			reply.Content = append(reply.Content, struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{frame.ContentBlock.Type, frame.ContentBlock.Text})
			if frame.ContentBlock.Type == "text" && frame.ContentBlock.Text != "" {
				heard = true
				listen.text(frame.ContentBlock.Text)
			}
		case "content_block_delta":
			if frame.Index < 0 || frame.Index >= len(reply.Content) {
				return renderErr("the API's answer arrived out of order")
			}
			switch frame.Delta.Type {
			case "text_delta":
				reply.Content[frame.Index].Text += frame.Delta.Text
				if frame.Delta.Text != "" {
					heard = true
					listen.text(frame.Delta.Text)
				}
			case "thinking_delta":
				listen.thinking(len(frame.Delta.Thinking))
			}
		case "message_delta":
			if frame.Delta.StopReason != "" {
				reply.StopReason = frame.Delta.StopReason
			}
			if frame.Usage.OutputTokens > 0 {
				reply.Usage.OutputTokens = frame.Usage.OutputTokens
			}
		case "message_stop":
			done = true
		case "error":
			return &streamError{frame.Error.Type, frame.Error.Message}
		}
		return nil
	})
	if err != nil {
		return nil, heard, err
	}
	if !done {
		return nil, heard, renderErr("the API's answer stopped before it was finished")
	}
	return &reply, heard, nil
}

// localChunk is one piece of llama-server's streamed answer.
type localChunk struct {
	Choices []struct {
		Delta struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Timings  *localTimings `json:"timings"`
	Progress *struct {
		Total     int `json:"total"`
		Cache     int `json:"cache"`
		Processed int `json:"processed"`
	} `json:"prompt_progress"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// localTimings is what llama-server measured, in milliseconds and tokens.
type localTimings struct {
	PromptN            int     `json:"prompt_n"`
	PromptMS           float64 `json:"prompt_ms"`
	PromptPerSecond    float64 `json:"prompt_per_second"`
	PredictedN         int     `json:"predicted_n"`
	PredictedMS        float64 `json:"predicted_ms"`
	PredictedPerSecond float64 `json:"predicted_per_second"`
}

// localAnswer is llama-server's streamed answer put back together.
type localAnswer struct {
	Content      string        `json:"content"`
	Reasoning    int           `json:"reasoning_chars"`
	FinishReason string        `json:"finish_reason"`
	PromptTokens int           `json:"prompt_tokens"`
	Written      int           `json:"completion_tokens"`
	Timings      *localTimings `json:"timings,omitempty"`
}

// readLocalStream reads llama-server's answer as it is written.
func readLocalStream(r io.Reader, listen *Listener) (*localAnswer, error) {
	var answer localAnswer
	var text strings.Builder
	finished := false
	err := sseLines(r, func(_, data string) error {
		if strings.TrimSpace(data) == "[DONE]" {
			finished = true
			return nil
		}
		var chunk localChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return renderErr("the local model sent a piece of its answer that is not JSON")
		}
		if chunk.Error != nil {
			return renderErr("the local model reported an error: %s", Scrub(chunk.Error.Message, 400))
		}
		if p := chunk.Progress; p != nil {
			// What was already in the cache from an earlier request is not
			// read again, so it is neither done nor to do.
			listen.reading(max(p.Processed-p.Cache, 0), max(p.Total-p.Cache, 0))
		}
		for _, choice := range chunk.Choices {
			if c := choice.Delta.Content; c != nil {
				text.WriteString(*c)
				listen.text(*c)
			}
			if r := choice.Delta.ReasoningContent; r != nil {
				answer.Reasoning += len(*r)
				listen.thinking(len(*r))
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				answer.FinishReason = *choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			answer.PromptTokens = chunk.Usage.PromptTokens
			answer.Written = chunk.Usage.CompletionTokens
		}
		if chunk.Timings != nil {
			answer.Timings = chunk.Timings
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The last word is either the end marker or a reason to stop. Neither
	// means the connection went before the answer was done.
	if !finished && answer.FinishReason == "" {
		return nil, renderErr("the local model's answer stopped before it was finished")
	}
	answer.Content = text.String()
	if answer.Timings != nil {
		if answer.PromptTokens == 0 {
			answer.PromptTokens = answer.Timings.PromptN
		}
		if answer.Written == 0 {
			answer.Written = answer.Timings.PredictedN
		}
	}
	return &answer, nil
}

// asStreamError says whether an error is one the server sent in the middle
// of an answer.
func asStreamError(err error) (*streamError, bool) {
	var s *streamError
	ok := errors.As(err, &s)
	return s, ok
}

// ---------------------------------------------------------------------------
// Clips as they arrive
// ---------------------------------------------------------------------------

// clipScanner finds each clip in a plan answer the moment its closing brace
// arrives, without waiting for the answer around it to finish.
//
// A clip is an object directly inside the list named "clips" of an object
// that is not inside anything. That is the whole shape of a plan, and it is
// the only place a clip can be: a brace inside a title, an object inside
// something else, or an answer that begins with a sentence are all read past.
// What it hands out is text, still untrusted, and goes through the same
// checks as a whole answer does.
type clipScanner struct {
	buf      strings.Builder
	at       int
	stack    []byte
	inString bool
	escaped  bool
	// key is the string last read directly inside the outer object, and
	// quoted is whether the one being read now is such a string.
	key       strings.Builder
	quoted    bool
	lastKey   string
	clipsOpen bool
	start     int
}

// feed takes the next piece of the answer and gives back every clip that
// was completed by it, in order.
func (s *clipScanner) feed(piece string) []string {
	s.buf.WriteString(piece)
	text := s.buf.String()
	var found []string
	for ; s.at < len(text); s.at++ {
		ch := text[s.at]
		if s.inString {
			if s.quoted && !(s.escaped || ch == '\\' || ch == '"') {
				s.key.WriteByte(ch)
			}
			switch {
			case s.escaped:
				s.escaped = false
			case ch == '\\':
				s.escaped = true
			case ch == '"':
				s.inString = false
				if s.quoted {
					s.lastKey = s.key.String()
					s.quoted = false
				}
			}
			continue
		}
		switch ch {
		case '"':
			s.inString = true
			// Only the strings directly inside the outer object can be its
			// keys. Anything deeper is a value or a key of something else.
			if len(s.stack) == 1 && s.stack[0] == '{' {
				s.quoted = true
				s.key.Reset()
			}
		case '{', '[':
			if len(s.stack) == 0 && ch != '{' {
				// A list that stands on its own is not a plan.
				s.stack = append(s.stack, ch)
				continue
			}
			if len(s.stack) == 1 && ch == '[' && s.stack[0] == '{' {
				s.clipsOpen = s.lastKey == "clips"
			}
			if len(s.stack) == 2 && ch == '{' && s.clipsOpen {
				s.start = s.at
			}
			s.stack = append(s.stack, ch)
		case '}', ']':
			if len(s.stack) == 0 {
				continue
			}
			opener := s.stack[len(s.stack)-1]
			if (opener == '{') != (ch == '}') {
				// Brackets that do not match: the answer is broken from
				// here, and nothing more is read out of it as a clip.
				s.stack = s.stack[:0]
				s.clipsOpen = false
				continue
			}
			s.stack = s.stack[:len(s.stack)-1]
			if len(s.stack) == 2 && ch == '}' && s.clipsOpen {
				found = append(found, text[s.start:s.at+1])
			}
			if len(s.stack) == 1 {
				s.clipsOpen = false
			}
			if len(s.stack) == 0 {
				s.lastKey = ""
			}
		}
	}
	return found
}
