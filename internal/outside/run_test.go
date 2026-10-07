//go:build outside && darwin

package outside

// How each step in sequences.go is done on a Mac, against the app's probe,
// cmd/framefairy-app/probe_outside.go. Only make outside runs this, in CI.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Where the probe in the app listens, probeAddr in probe_outside.go.
const probe = "http://127.0.0.1:47290"

// The local dispenser's address, the one make dispenser uses.
const dispenser = "127.0.0.1:8090"

// The speech model the setup looks for, the first in engine.SpeechModels.
const speechModel = "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// How long a check waits for what it is about, and how long a closed app
// is given to start.
const (
	checkWithin = 20 * time.Second
	startWithin = 90 * time.Second
)

// state is what the probe answers, ProbeState there.
type state struct {
	Links   int `json:"links"`
	Licence struct {
		Saved   bool   `json:"saved"`
		About   string `json:"about"`
		Waiting bool   `json:"waiting"`
	} `json:"licence"`
	Focused bool `json:"focused"`
	Page    struct {
		Settings bool   `json:"settings"`
		Key      string `json:"key"`
		Line     string `json:"line"`
		Mark     string `json:"mark"`
	} `json:"page"`
	Problem string `json:"problem"`
}

var client = &http.Client{Timeout: 10 * time.Second}

func TestSequences(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("only in CI: on a Mac of one's own this replaces the licence in the keychain")
	}
	app := os.Getenv("FRAMEFAIRY_OUTSIDE_APP")
	disp := os.Getenv("FRAMEFAIRY_OUTSIDE_DISPENSER")
	if app == "" || disp == "" {
		t.Fatal("make outside says where the app and the dispenser are")
	}
	run(t, lsregister, "-f", app)
	m := &mac{dispenserProgram: disp}
	t.Cleanup(func() {
		if t.Failed() {
			showLog(t)
		}
		m.quit()
		if m.stopDispenser != nil {
			m.stopDispenser()
		}
	})
	for _, seq := range sequences {
		if !t.Run(seq.name, func(t *testing.T) { m.play(t, seq) }) {
			return
		}
	}
}

// mac is what lasts from one sequence to the next: the dispenser, once
// started, and the keys the sequence bought.
type mac struct {
	dispenserProgram string
	stopDispenser    func()
	bought           int
	keys             []string
}

// play runs a sequence's steps in order. The first that fails ends it,
// named as it is written. One that passes leaves a notice on the check
// with every step, so the pull request says what was done.
func (m *mac) play(t *testing.T, seq sequence) {
	m.keys = nil
	m.quit()
	for i, s := range seq.steps {
		do, ok := done[s[0]]
		if !ok {
			t.Fatalf("step %d, %v: no such verb", i+1, s)
		}
		if err := do(m, s[1:]); err != nil {
			say("error", seq.name, fmt.Sprintf("step %d, %v: %v", i+1, s, err))
			t.Fatalf("step %d, %v: %v", i+1, s, err)
		}
		t.Logf("step %d, %v", i+1, s)
	}
	lines := make([]string, len(seq.steps))
	for i, s := range seq.steps {
		lines[i] = fmt.Sprintf("%d. %v", i+1, s)
	}
	say("notice", seq.name, "passed:\n"+strings.Join(lines, "\n"))
}

// done is how each verb in sequences.go is done.
var done = map[string]func(m *mac, args []string) error{
	"set up":        (*mac).setUp,
	"buy":           (*mac).buy,
	"quit":          func(m *mac, _ []string) error { return m.quit() },
	"open link":     (*mac).openLink,
	"bring forward": (*mac).bringForward,
	"click":         (*mac).click,
	"front": func(m *mac, a []string) error {
		return within(checkWithin, func() (bool, string) {
			f := front()
			return strings.Contains(f, `"`+a[0]+`"`), "in front: " + f
		})
	},
	"field": func(m *mac, a []string) error {
		want := ""
		if a[0] != "empty" {
			want = m.key(a[0])
		}
		return m.check(func(s state) bool { return s.Page.Settings && s.Page.Key == want })
	},
	"line": func(m *mac, a []string) error {
		return m.check(func(s state) bool { return s.Page.Line == a[0] })
	},
	"line has": func(m *mac, a []string) error {
		return m.check(func(s state) bool { return strings.Contains(s.Page.Line, a[0]) })
	},
	"mark": func(m *mac, a []string) error {
		want := a[0]
		if want == "none" {
			want = ""
		}
		return m.check(func(s state) bool { return s.Page.Mark == want })
	},
	"saved": func(m *mac, a []string) error {
		return m.check(func(s state) bool { return s.Licence.Saved == (a[0] == "yes") })
	},
	"waiting": func(m *mac, a []string) error {
		return m.check(func(s state) bool { return s.Licence.Waiting == (a[0] == "yes") })
	},
}

// Every verb sequences.go knows is one the runner can do.
func TestEveryVerbIsDone(t *testing.T) {
	for v := range verbs {
		if done[v] == nil {
			t.Errorf("no way to do %q", v)
		}
	}
}

// setUp makes this Mac one whose setup is done, the way Tim's is.
func (m *mac) setUp(_ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	model := filepath.Join(home, ".framefairy", "models", speechModel)
	if err := os.MkdirAll(model, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(model, "tokens.txt"), []byte("<blk> 0\n"), 0o644); err != nil {
		return err
	}
	config := filepath.Join(home, "Library", "Application Support", "FrameFairy")
	if err := os.MkdirAll(config, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"chosen": true}`+"\n"), 0o644)
}

var unlockLink = regexp.MustCompile(`href="framefairy://unlock\?key=([^"]+)"`)

// buy buys seats at the local dispenser, started the first time, and adds
// the keys in the letter to the sequence's.
func (m *mac) buy(a []string) error {
	seats, _ := strconv.Atoi(a[0])
	if m.stopDispenser == nil {
		if err := m.startDispenser(); err != nil {
			return err
		}
	}
	base := "http://" + dispenser
	before, err := lettersKeys(base)
	if err != nil {
		return err
	}
	m.bought++
	noRedirect := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	email := fmt.Sprintf("outside%d@example.com", m.bought)
	resp, err := noRedirect.PostForm(base+"/shop/buy", url.Values{"email": {email}, "seats": {a[0]}})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("the checkout answered %d", resp.StatusCode)
	}
	var fresh []string
	err = within(60*time.Second, func() (bool, string) {
		all, err := lettersKeys(base)
		if err != nil {
			return false, err.Error()
		}
		fresh = fresh[:0]
		for _, k := range all {
			if !contains(before, k) {
				fresh = append(fresh, k)
			}
		}
		return len(fresh) == seats, fmt.Sprintf("%d new keys in the letters", len(fresh))
	})
	m.keys = append(m.keys, fresh...)
	return err
}

func lettersKeys(base string) ([]string, error) {
	resp, err := client.Get(base + "/dev")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	page, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, m := range unlockLink.FindAllStringSubmatch(string(page), -1) {
		if !contains(keys, m[1]) {
			keys = append(keys, m[1])
		}
	}
	return keys, nil
}

func (m *mac) startDispenser() error {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, m.dispenserProgram, "dev", "-addr", dispenser)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	m.stopDispenser = func() {
		cancel()
		_ = cmd.Wait()
	}
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(out)
		said := false
		for sc.Scan() {
			fmt.Fprintf(os.Stderr, "dispenser: %s\n", sc.Text())
			if !said && strings.HasPrefix(sc.Text(), "The dispenser runs at") {
				said = true
				close(ready)
			}
		}
	}()
	select {
	case <-ready:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("the dispenser did not start")
	}
}

func (m *mac) key(n string) string {
	k, _ := strconv.Atoi(n)
	if k < 1 || k > len(m.keys) {
		return "no key " + n
	}
	return m.keys[k-1]
}

// openLink hands a key's link to macOS, as a browser does once a link is
// clicked, and waits until the app has been handed it: one link more than
// it had, or the first, when the link has to start it.
func (m *mac) openLink(a []string) error {
	link := "framefairy://unlock?key=" + m.key(a[0])
	if len(a) > 1 {
		link += a[1]
	}
	want, wait := 1, startWithin
	if s, err := ask(); err == nil {
		want, wait = s.Links+1, checkWithin
	}
	if out, err := exec.Command("open", link).CombinedOutput(); err != nil {
		return fmt.Errorf("open: %v: %s", err, out)
	}
	return within(wait, func() (bool, string) {
		s, err := ask()
		if err != nil {
			return false, err.Error()
		}
		return s.Links >= want, fmt.Sprintf("the app was handed %d links", s.Links)
	})
}

func (m *mac) bringForward(a []string) error {
	if out, err := exec.Command("open", "-a", a[0]).CombinedOutput(); err != nil {
		return fmt.Errorf("open -a: %v: %s", err, out)
	}
	return within(checkWithin, func() (bool, string) {
		f := front()
		return strings.Contains(f, `"`+a[0]+`"`), "in front: " + f
	})
}

func (m *mac) click(a []string) error {
	return within(checkWithin, func() (bool, string) {
		resp, err := client.Post(probe+"/click?button="+url.QueryEscape(a[0]), "text/plain", nil)
		if err != nil {
			return false, err.Error()
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode == http.StatusNoContent, strings.TrimSpace(string(b))
	})
}

// quit quits the app if it runs, and waits until it is gone.
func (m *mac) quit() error {
	resp, err := client.Post(probe+"/quit", "text/plain", nil)
	if err != nil {
		return nil
	}
	resp.Body.Close()
	return within(checkWithin, func() (bool, string) {
		_, err := ask()
		return err != nil, "the app still answers"
	})
}

// check waits until what the probe says holds.
func (m *mac) check(ok func(state) bool) error {
	return within(checkWithin, func() (bool, string) {
		s, err := ask()
		if err != nil {
			return false, err.Error()
		}
		return ok(s), fmt.Sprintf("the app says %+v", s)
	})
}

// within asks until the answer is yes, and fails with the last thing it
// was told when that does not happen in time.
func within(d time.Duration, try func() (bool, string)) error {
	deadline := time.Now().Add(d)
	var last string
	for {
		ok, said := try()
		if ok {
			return nil
		}
		last = said
		if time.Now().After(deadline) {
			return fmt.Errorf("not within %s. Last: %s", d, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func ask() (state, error) {
	var s state
	resp, err := client.Get(probe + "/state")
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&s)
	return s, err
}

// front is the name of the app in front, from Launch Services, which
// needs no permission the runner does not have.
func front() string {
	asn, err := exec.Command("lsappinfo", "front").Output()
	if err != nil {
		return ""
	}
	info, err := exec.Command("lsappinfo", "info", "-only", "name", strings.TrimSpace(string(asn))).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(info))
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// say leaves a notice or an error on the CI check, which shows on the
// pull request and can be read without the job's log.
func say(level, title, text string) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		return
	}
	esc := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	prop := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	fmt.Printf("::%s title=%s::%s\n", level, prop.Replace(title), esc.Replace(text))
}

func showLog(t *testing.T) {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, "Library", "Logs", "FrameFairy-outside.log"))
	if err != nil {
		t.Logf("no log from the app: %v", err)
		return
	}
	t.Logf("the app's log:\n%s", b)
}
