//go:build outside && darwin

package outside

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Where the probe in the app listens, probeAddr in
// cmd/framefairy-app/probe_outside.go.
const probe = "http://127.0.0.1:47290"

// The local dispenser's address, the one make dispenser uses.
const dispenser = "127.0.0.1:8090"

// The speech model the setup looks for, the first in engine.SpeechModels.
const speechModel = "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// pageView and state are what the probe answers, PageView and ProbeState
// there.
type pageView struct {
	Settings  bool   `json:"settings"`
	Key       string `json:"key"`
	Line      string `json:"line"`
	Mark      string `json:"mark"`
	Unlock    string `json:"unlock"`
	UnlockOff bool   `json:"unlockOff"`
}

type state struct {
	Links   int `json:"links"`
	Licence struct {
		Saved   bool   `json:"saved"`
		About   string `json:"about"`
		Waiting bool   `json:"waiting"`
	} `json:"licence"`
	Focused bool     `json:"focused"`
	Page    pageView `json:"page"`
	Problem string   `json:"problem"`
}

var client = &http.Client{Timeout: 10 * time.Second}

// The Unlock button in the letter a buyer gets, with the app closed and
// with it open, the steps Tim took by hand for #95.
func TestTheUnlockLinkInTheLetter(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("only in CI: on a Mac of one's own this replaces the licence in the keychain")
	}
	app := os.Getenv("FRAMEFAIRY_OUTSIDE_APP")
	disp := os.Getenv("FRAMEFAIRY_OUTSIDE_DISPENSER")
	if app == "" || disp == "" {
		t.Fatal("make outside says where the app and the dispenser are")
	}
	setUp(t)
	t.Cleanup(func() {
		if t.Failed() {
			showLog(t)
		}
		quitApp(t)
	})

	keys := buyTwoSeats(t, disp)
	run(t, lsregister, "-f", app)
	if alive() {
		t.Fatal("the app is already running")
	}

	// 1. The app is closed. The first key's Unlock opens it on the
	// settings, with the key in the field, and nothing unlocked yet.
	open(t, link(keys[0]))
	s := waitFor(t, 90*time.Second, "the app opened on the settings with the first key", func(s state) bool {
		return s.Page.Settings && s.Page.Key == keys[0] && s.Page.Line == "From the link. Unlock takes it."
	})
	if s.Links != 1 || s.Licence.Saved {
		t.Fatalf("after the first link: %+v", s)
	}
	waitFront(t, "Frame Fairy")

	// 2. Unlock takes it.
	click(t, "button.act.unlock")
	s = waitFor(t, 20*time.Second, "the key unlocked", func(s state) bool {
		return s.Licence.Saved && s.Page.Mark == "ok" && strings.Contains(s.Page.Line, "a test key")
	})
	if !strings.HasPrefix(s.Page.Line, "Key ") || s.Page.Key != "" {
		t.Fatalf("after Unlock: %+v", s.Page)
	}

	// 3. Another app in front, and the second key's Unlock. The app comes
	// back to the front with the second key in the field.
	run(t, "open", "-a", "Calculator")
	waitFront(t, "Calculator")
	open(t, link(keys[1]))
	s = waitFor(t, 20*time.Second, "the second key in the field", func(s state) bool {
		return s.Links == 2 && s.Page.Key == keys[1] && s.Page.Line == "From the link. Unlock takes it."
	})
	waitFront(t, "Frame Fairy")

	// 4. A link with more in it than one key changes nothing.
	open(t, link(keys[0])+"&more=1")
	s = waitFor(t, 20*time.Second, "the third link counted", func(s state) bool { return s.Links == 3 })
	time.Sleep(time.Second)
	s = read(t)
	if s.Page.Key != keys[1] || s.Licence.Waiting {
		t.Fatalf("a link with more in it changed the settings: %+v", s)
	}
}

// setUp makes the runner a Mac that has been set up, the way Tim's is:
// a speech model in place and the question how clips are found answered.
// Without them the app holds a link until the setup is done.
func setUp(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(home, ".framefairy", "models", speechModel)
	if err := os.MkdirAll(model, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(model, "tokens.txt"), []byte("<blk> 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "Library", "Application Support", "FrameFairy")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(`{"chosen": true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buyTwoSeats starts the local dispenser, buys two seats at its checkout
// and reads the Unlock links out of the letter on its dev page.
func buyTwoSeats(t *testing.T, disp string) []string {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, disp, "dev", "-addr", dispenser)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(out)
		said := false
		for sc.Scan() {
			t.Logf("dispenser: %s", sc.Text())
			if !said && strings.HasPrefix(sc.Text(), "The dispenser runs at") {
				said = true
				close(ready)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("the dispenser did not start")
	}

	base := "http://" + dispenser
	noRedirect := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := noRedirect.PostForm(base+"/shop/buy", url.Values{"email": {"outside@example.com"}, "seats": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the checkout answered %d", resp.StatusCode)
	}

	unlock := regexp.MustCompile(`href="framefairy://unlock\?key=([^"]+)"`)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/dev")
		if err == nil {
			page, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var keys []string
			for _, m := range unlock.FindAllStringSubmatch(string(page), -1) {
				keys = append(keys, m[1])
			}
			if len(keys) == 2 {
				return keys
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("no letter with two Unlock links on the dev page")
	return nil
}

func link(key string) string { return "framefairy://unlock?key=" + key }

// open hands a link to macOS, as a browser or a mail app does once a
// link is clicked.
func open(t *testing.T, link string) {
	t.Helper()
	t.Logf("open %s…", link[:min(len(link), 40)])
	run(t, "open", link)
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func alive() bool {
	resp, err := client.Get(probe + "/state")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func read(t *testing.T) state {
	t.Helper()
	s, err := ask()
	if err != nil {
		t.Fatal(err)
	}
	return s
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

// waitFor asks the probe until what is asked for holds, and fails with
// the last answer when it does not in time.
func waitFor(t *testing.T, within time.Duration, what string, ok func(state) bool) state {
	t.Helper()
	deadline := time.Now().Add(within)
	var last state
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = ask()
		if lastErr == nil && ok(last) {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("not %s within %s. Last answer: %+v %v", what, within, last, lastErr)
	return last
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
	return string(info)
}

func waitFront(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(front(), `"`+name+`"`) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s is not in front: %s", name, front())
}

func click(t *testing.T, selector string) {
	t.Helper()
	resp, err := client.Post(probe+"/click?selector="+url.QueryEscape(selector), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("click %s: %d", selector, resp.StatusCode)
	}
}

func quitApp(t *testing.T) {
	resp, err := client.Post(probe+"/quit", "text/plain", nil)
	if err != nil {
		return
	}
	resp.Body.Close()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && alive() {
		time.Sleep(250 * time.Millisecond)
	}
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
