//go:build outside && darwin

package outside

// How each step in sequences.go is done on a Mac, against the app's probe,
// cmd/framefairy-app/probe_outside.go. Only make outside runs this, in CI.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Where the probe in the app listens, probeAddr in probe_outside.go.
const probe = "http://127.0.0.1:47290"

// Where the test hears that the probe is up, probeHello there.
const hello = "127.0.0.1:47291"

// The local dispenser's address, the one make dispenser uses.
const dispenser = "127.0.0.1:8090"

// The speech model the setup looks for, the first in engine.SpeechModels.
const speechModel = "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// No check here waits on the clock. Each waits for what it is about to be
// said: the probe saying the app is up, that something changed, or the
// app's connection ending as it goes. A check that never holds is ended by
// go test's -timeout in make outside, and what the app said last is on
// the way to it, see until.

// state is what the probe answers, ProbeState there.
type state struct {
	Version int `json:"version"`
	Links   int `json:"links"`
	Licence struct {
		Saved   bool   `json:"saved"`
		About   string `json:"about"`
		Waiting bool   `json:"waiting"`
	} `json:"licence"`
	Focused bool `json:"focused"`
	Page    struct {
		Settings bool   `json:"settings"`
		Head     string `json:"head"`
		Key      string `json:"key"`
		Line     string `json:"line"`
		Mark     string `json:"mark"`
	} `json:"page"`
	Searches []search `json:"searches"`
	Windows  []string `json:"windows"`
	Problem  string   `json:"problem"`
}

// search is one search the app knows of, SearchView there.
type search struct {
	Episode string  `json:"episode"`
	State   string  `json:"state"`
	Step    string  `json:"step"`
	From    float64 `json:"from"`
	To      float64 `json:"to"`
	Error   string  `json:"error"`
}

// client asks the probe and the dispenser. With no limit of its own: an
// ask for the state after a change waits for the change.
var client = &http.Client{}

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
	m := &mac{dispenserProgram: disp, app: app, ffmpeg: filepath.Join(filepath.Dir(disp), "ffmpeg"), up: readyAt(t)}
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
	// The app and the ffmpeg make outside put beside the dispenser.
	app    string
	ffmpeg string
	// video is the video added last.
	video         string
	stopDispenser func()
	bought        int
	keys          []string
	// up hears each time the app's probe is up, see readyAt.
	up <-chan struct{}
}

// readyAt listens where the probe says it is up, and hands each hello on.
func readyAt(t *testing.T) <-chan struct{} {
	ln, err := net.Listen("tcp", hello)
	if err != nil {
		t.Fatal(err)
	}
	up := make(chan struct{}, 8)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		up <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return up
}

// opening drains what was heard before, so the next hello is the app's
// that is opened now.
func (m *mac) opening() {
	for len(m.up) > 0 {
		<-m.up
	}
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
	"front":         func(m *mac, a []string) error { return inFront(a[0]) },
	"head": func(m *mac, a []string) error {
		return m.check(func(s state) bool { return s.Page.Head == a[0] })
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
	"windows": func(m *mac, a []string) error {
		want := strings.Split(a[0], ", ")
		return m.check(func(s state) bool {
			have := slices.Clone(s.Windows)
			slices.Sort(have)
			slices.Sort(want)
			return slices.Equal(have, want)
		})
	},
	"close": func(m *mac, a []string) error {
		resp, err := client.Post(probe+"/close?window="+url.QueryEscape(a[0]), "text/plain", nil)
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			return errors.New(strings.TrimSpace(string(b)))
		}
		return nil
	},
	"start":           (*mac).start,
	"add video":       (*mac).addVideo,
	"search and quit": (*mac).searchAndQuit,
	"search ends": func(m *mac, _ []string) error {
		return m.check(func(s state) bool {
			for _, j := range s.Searches {
				if j.Episode == m.video && (j.State == "running" || j.State == "queued") {
					return false
				}
			}
			return true
		})
	},
	"search": func(m *mac, a []string) error {
		from, _ := strconv.ParseFloat(a[1], 64)
		to, _ := strconv.ParseFloat(a[2], 64)
		return m.check(func(s state) bool {
			var last *search
			for i, j := range s.Searches {
				if j.Episode == m.video {
					last = &s.Searches[i]
				}
			}
			return last != nil && last.State == a[0] && last.From == from && last.To == to
		})
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

// setUp makes this Mac one whose setup is done: a speech model, and clips
// found in the cloud, so the app opens on its workspace with no model of
// its own to find clips with. A model on this Mac would be one more
// program the app starts, which the licence is not about.
//
// On the real disk: the built app reads its settings and the speech model from this Mac's home folder.
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
	return os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"chosen": true, "planner": "api"}`+"\n"), 0o644)
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
	// The checkout sends Paddle's webhook on its own. Deliver on the dev
	// page sends what is still due and answers once it is answered, and the
	// dispenser sends the letter while it answers the webhook.
	resp, err = noRedirect.PostForm(base+"/dev/deliver", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("deliver on the dev page answered %d", resp.StatusCode)
	}
	all, err := lettersKeys(base)
	if err != nil {
		return err
	}
	for _, k := range all {
		if !contains(before, k) {
			fresh = append(fresh, k)
		}
	}
	if len(fresh) != seats {
		return fmt.Errorf("%d new keys in the letters, not %d", len(fresh), seats)
	}
	m.keys = append(m.keys, fresh...)
	return nil
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
	<-ready
	return nil
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
	want, closed := 1, true
	if s, err := ask(-1); err == nil {
		want, closed = s.Links+1, false
	}
	m.opening()
	if out, err := exec.Command("open", link).CombinedOutput(); err != nil {
		return fmt.Errorf("open: %v: %s", err, out)
	}
	if closed {
		<-m.up
	}
	return until(func(s state) bool { return s.Links >= want })
}

// start starts the app the way the Dock does, and waits until it answers.
func (m *mac) start(_ []string) error {
	m.opening()
	if out, err := exec.Command("open", m.app).CombinedOutput(); err != nil {
		return fmt.Errorf("open: %v: %s", err, out)
	}
	<-m.up
	return nil
}

// addVideo makes a video of a test picture and a tone, with the ffmpeg the
// app carries, and adds it the way Add does once a file is picked.
//
// On the real disk: ffmpeg makes the video and the built app reads it.
func (m *mac) addVideo(a []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Movies", "outside")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	video := filepath.Join(dir, fmt.Sprintf("episode-%d.mp4", time.Now().UnixNano()))
	out, err := exec.Command(m.ffmpeg, "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d="+a[0],
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d="+a[0],
		"-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", video).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making the video: %v: %s", err, out)
	}
	if err := post("/add?video=" + url.QueryEscape(video)); err != nil {
		return err
	}
	m.video = video
	return nil
}

// searchAndQuit asks for a search and quits the app in the same moment,
// and waits until it is gone.
func (m *mac) searchAndQuit(a []string) error {
	q := url.Values{"video": {m.video}, "from": {a[0]}, "to": {a[1]}, "quit": {"1"}}
	gone, err := goneOnce()
	if err != nil {
		return err
	}
	if err := post("/search?" + q.Encode()); err != nil {
		return err
	}
	gone()
	return nil
}

// goneOnce opens the probe's answer that lasts as long as the app, and
// gives back what waits for it to end. It fails when the app is not
// running.
func goneOnce() (wait func(), err error) {
	resp, err := client.Get(probe + "/gone")
	if err != nil {
		return nil, err
	}
	return func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}, nil
}

// post asks the probe for something and says what it answered, unless
// that was a yes.
func post(path string) error {
	resp, err := client.Post(probe+path, "text/plain", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("the probe answered %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (m *mac) bringForward(a []string) error {
	if out, err := exec.Command("open", "-a", a[0]).CombinedOutput(); err != nil {
		return fmt.Errorf("open -a: %v: %s", err, out)
	}
	return inFront(a[0])
}

// inFront waits until the app named is the one in front. Launch Services
// says which is, and the app's own window coming to the front or going to
// the back is what says when to ask it again: a change of the app in front
// is always one of the two. The app is running in every sequence that asks.
func inFront(name string) error {
	return until(func(state) bool {
		f := front()
		if !strings.Contains(f, `"`+name+`"`) {
			saw("in front: " + f)
			return false
		}
		return true
	})
}

// click presses a button once one on screen says its words, and asks again
// each time the app says something changed.
func (m *mac) click(a []string) error {
	after := -1
	for {
		s, err := ask(after)
		if err != nil {
			return err
		}
		resp, err := client.Post(probe+"/click?button="+url.QueryEscape(a[0]), "text/plain", nil)
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		saw(strings.TrimSpace(string(b)))
		after = s.Version
	}
}

// quit quits the app if it runs, and waits until it is gone.
func (m *mac) quit() error {
	gone, err := goneOnce()
	if err != nil {
		return nil
	}
	resp, err := client.Post(probe+"/quit", "text/plain", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	gone()
	return nil
}

// check waits until what the probe says holds.
func (m *mac) check(ok func(state) bool) error { return until(ok) }

// until asks the probe what the app has until ok holds. Between asks it
// waits for the probe to say something changed, never on the clock. What
// the app has is said each time it differs from the time before, so a
// check that never holds shows what the app had last when go test's
// -timeout ends it.
func until(ok func(state) bool) error {
	after := -1
	for {
		s, err := ask(after)
		if err != nil {
			return err
		}
		if ok(s) {
			return nil
		}
		was := s
		was.Version = 0
		saw(fmt.Sprintf("the app says %+v", was))
		after = s.Version
	}
}

// ask is what the probe says the app has: at once with -1, or once its
// version is past after.
func ask(after int) (state, error) {
	var s state
	resp, err := client.Get(probe + "/state?after=" + strconv.Itoa(after))
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&s)
	return s, err
}

// seen is the last thing saw wrote, so the same is not written twice.
var seen string

// saw writes what a check saw that did not hold yet, once for each time it
// differs, for the log of a run that hangs.
func saw(what string) {
	if what == seen {
		return
	}
	seen = what
	fmt.Fprintf(os.Stderr, "outside: %s\n", what)
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

// On the real disk: the log is what the built app wrote on this Mac.
func showLog(t *testing.T) {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, "Library", "Logs", "FrameFairy-outside.log"))
	if err != nil {
		t.Logf("no log from the app: %v", err)
		return
	}
	t.Logf("the app's log:\n%s", b)
}
