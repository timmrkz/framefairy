package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// An answer without JSON
//
// A plain answer is one line a clip: its line numbers, the title and the
// reason, split by "|", as in
//
//	12 18 19 | Der Regenschirm | Ein Kind wehrt sich mit Judo.
//
// JSON spends about a third of an answer on keys, quotes and brackets. A
// local model is held to the line by a grammar, the way it is held to JSON
// by a schema, and each line is read into the same clip object a JSON
// answer gives, so everything after the reading is the same. What a model
// writes is untrusted: a line that is not a clip is passed over, and the
// clip goes through the checks every clip goes through.
// ---------------------------------------------------------------------------

// plainClip reads one line of a plain answer of points into the clip
// object a JSON answer has, and says false for a line that is no clip:
// prose, a blank, too few numbers.
func plainClip(line string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
	numbers := strings.Fields(parts[0])
	if len(parts) < 2 || len(numbers) != 3 {
		return "", false
	}
	var n [3]int
	for i, field := range numbers {
		v, err := strconv.Atoi(field)
		if err != nil || v < 1 {
			return "", false
		}
		n[i] = v
	}
	clip := map[string]any{"start": n[0], "payoff": n[1], "end": n[2],
		"title": strings.TrimSpace(parts[1]), "reason": ""}
	if len(parts) == 3 {
		clip["reason"] = strings.TrimSpace(parts[2])
	}
	body, err := json.Marshal(clip)
	if err != nil {
		return "", false
	}
	return string(body), true
}

// plainAnswer is a whole plain answer as the JSON object a JSON answer
// is, {"clips": [...]}, every line that is a clip in it, in order.
func plainAnswer(reply string) string {
	var clips []string
	for _, line := range strings.Split(reply, "\n") {
		if clip, ok := plainClip(line); ok {
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

// pointsGrammar holds a local model to at most count lines of a plain
// answer of points, in llama.cpp's grammar notation: three numbers, the
// title and the reason.
func pointsGrammar(_, count int) string {
	return fmt.Sprintf(`root ::= clip ("\n" clip){0,%d} "\n"?
clip ::= number " " number " " number " | " text " | " text
number ::= [1-9] [0-9]{0,5}
text ::= [^|\n]+
`, max(count, 1)-1)
}
