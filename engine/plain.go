package engine

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// An answer without JSON
//
// A plain answer is one line a clip. points writes its three line numbers,
// the title and the reason, split by "|", and middle only its three line
// numbers, the start, a line in the middle and the end:
//
//	12 18 19 | Der Regenschirm | Ein Kind wehrt sich mit Judo.
//	31 40 52
//
// JSON spends about a third of an answer on keys, quotes and brackets.
// Each line is read into the same clip object a JSON answer gives, so
// everything after the reading is the same. The model is held to no
// grammar: llama-server applies a grammar of our own from the first token,
// before the model has thought, and a model that cannot open its thought
// writes it into the answer. What a model writes is untrusted: a line that
// is not a clip is passed over, the clips past the count asked for are
// left out, and every clip goes through the checks every clip goes
// through.
// ---------------------------------------------------------------------------

// plainFormat is how a recipe that answers without JSON is answered.
type plainFormat struct {
	// Clip reads one line of the answer into the clip object a JSON answer
	// has, and says false for a line that is no clip: prose, a blank, too
	// few numbers.
	Clip func(line string) (string, bool)
}

// pointsPlain is the answer of points: start, payoff and end, the title
// and the reason.
var pointsPlain = &plainFormat{
	Clip: func(line string) (string, bool) {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		n, ok := threeNumbers(parts[0])
		if len(parts) < 2 || !ok {
			return "", false
		}
		clip := map[string]any{"start": n[0], "payoff": n[1], "end": n[2],
			"title": strings.TrimSpace(parts[1]), "reason": ""}
		if len(parts) == 3 {
			clip["reason"] = strings.TrimSpace(parts[2])
		}
		return clipJSON(clip)
	},
}

// middlePlain is the answer of middle: the line a story starts on, a line
// in its middle and the line it ends on, in that order, and nothing else.
var middlePlain = &plainFormat{
	Clip: func(line string) (string, bool) {
		n, ok := threeNumbers(line)
		if !ok {
			return "", false
		}
		return clipJSON(map[string]any{"start": n[0], "middle": n[1], "end": n[2]})
	},
}

// threeNumbers reads a text that is three whole numbers of 1 or more.
func threeNumbers(text string) ([3]int, bool) {
	var n [3]int
	fields := strings.Fields(text)
	if len(fields) != 3 {
		return n, false
	}
	for i, field := range fields {
		v, err := strconv.Atoi(field)
		if err != nil || v < 1 {
			return n, false
		}
		n[i] = v
	}
	return n, true
}

func clipJSON(clip map[string]any) (string, bool) {
	body, err := json.Marshal(clip)
	if err != nil {
		return "", false
	}
	return string(body), true
}

// answer is a whole plain answer as the JSON object a JSON answer is,
// {"clips": [...]}, every line that is a clip in it, in order.
func (f *plainFormat) answer(reply string) string {
	var clips []string
	for _, line := range strings.Split(reply, "\n") {
		if clip, ok := f.Clip(line); ok {
			clips = append(clips, clip)
		}
	}
	return `{"clips": [` + strings.Join(clips, ", ") + `]}`
}

// lineScanner hands out each whole line of an answer as it is written.
type lineScanner struct {
	buf strings.Builder
}

// feed takes the next piece of the answer and gives back the lines it
// finished, in order.
func (s *lineScanner) feed(piece string) []string {
	s.buf.WriteString(piece)
	text := s.buf.String()
	cut := strings.LastIndexByte(text, '\n')
	if cut < 0 {
		return nil
	}
	s.buf.Reset()
	s.buf.WriteString(text[cut+1:])
	return strings.Split(text[:cut], "\n")
}
