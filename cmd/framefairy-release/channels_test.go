package main

import (
	"crypto/ed25519"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/updates"
)

// The channel list, written from what the release dev holds and what
// GitHub says of the open pull requests and main, the way publish.yml
// writes it as a build starts and when it ends.
//
// On the real disk: the release tool reads the zips and the entries it signs as files.
func TestTheChannelListIsWrittenFromWhatGitHubSays(t *testing.T) {
	private := release(t)
	public := private.Public().(ed25519.PublicKey)
	entry := func(channel, name string) []byte {
		data, err := os.ReadFile(signed(t, channel, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	head7 := "7777777777777777777777777777777777777777"
	mainHead := "9999999999999999999999999999999999999999"
	gh := &fakeGitHub{
		main:  mainHead,
		open:  []pull{{number: 7, title: "T", head: head7, branch: "b"}, {number: 8, title: "No build yet", head: "8", branch: "c"}},
		dirty: map[int]bool{7: true},
		runs: map[string][2]string{
			head7: {"in_progress", ""},
			// main's newest build failed, so none is coming, #840.
			mainHead: {"completed", "failure"},
		},
	}
	old := now.Add(-2 * time.Hour)
	gh.put("channel-main.json", entry("main", "main"), old)
	gh.put("channel-pr-7.json", entry("pr-7", "#7 T"), old)
	gh.put("channel-pr-9.json", entry("pr-9", "#9 Closed"), old)
	gh.put("main.zip", []byte("zip"), old)
	gh.put("pr-7.zip", []byte("zip"), old)
	gh.put("app-pr-9-old.zip", []byte("zip"), old)
	gh.put("app-pr-7-new.zip", []byte("zip"), now.Add(-10*time.Minute))
	gh.put("channels.json", []byte("{}"), old)
	gh.put("channels-a.json", []byte("{}"), now.Add(-time.Hour))
	gh.put("channels-b.json", []byte("{}"), old)
	trees := fakeTrees{"abc1234def56.." + head7: {"engine/run.go"}}

	if err := writeChannels(gh, trees, public, now, io.Discard); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"channels.json", "channels-b.json"} {
		l, err := updates.Parse(gh.data[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var channels []string
		for _, b := range l.Channels {
			channels = append(channels, b.Channel)
		}
		if strings.Join(channels, " ") != "main pr-7" {
			t.Errorf("%s lists %v, want main and pr-7, and not pr-9, which closed", name, channels)
		}
		if b, _ := l.Find("pr-7"); b.Newest != head7[:12] || !b.Conflict {
			t.Errorf("%s says pr-7 has %q coming and conflicts %v, want %s and true", name, b.Newest, b.Conflict, head7[:12])
		}
		if b, _ := l.Find("main"); b.Newest != "" {
			t.Errorf("%s says main has %s coming, whose build failed", name, b.Newest)
		}
		if !l.Written.Equal(now) {
			t.Errorf("%s says it was written at %v", name, l.Written)
		}
	}
	if _, ok := gh.release["channel-pr-9.json"]; ok {
		t.Error("the entry of pr-9, which is not open, is still there")
	}
	// Only the older copy is replaced, so the newer one stays where an app
	// reads it the whole time.
	if string(gh.data["channels-a.json"]) != "{}" {
		t.Error("channels-a.json, the newer copy, was replaced")
	}
	// A zip no entry points at goes after an hour, not before.
	if _, ok := gh.release["app-pr-9-old.zip"]; ok {
		t.Error("a zip nobody can reach, two hours old, is still there")
	}
	for _, kept := range []string{"app-pr-7-new.zip", "main.zip", "pr-7.zip"} {
		if _, ok := gh.release[kept]; !ok {
			t.Errorf("%s was taken away", kept)
		}
	}
	// The list replaces channels.json under a name of its own first.
	if i, j := slices.Index(gh.did, "upload channels.next.json"), slices.Index(gh.did, "rename channels.next.json channels.json"); i < 0 || j < i {
		t.Errorf("channels.json was not put in place by a rename: %v", gh.did)
	}
}

// When the open pull requests cannot be read, every entry stays: a
// failure to read them would otherwise take every pull request away.
//
// On the real disk: the release tool reads the zips and the entries it signs as files.
func TestTheListKeepsEveryEntryWhenThePullRequestsCannotBeRead(t *testing.T) {
	private := release(t)
	data, err := os.ReadFile(signed(t, "pr-7", "#7 T"))
	if err != nil {
		t.Fatal(err)
	}
	gh := &pullsDown{fakeGitHub: &fakeGitHub{}}
	gh.put("channel-pr-7.json", data, now)
	if err := writeChannels(gh, fakeTrees{}, private.Public().(ed25519.PublicKey), now, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, ok := gh.release["channel-pr-7.json"]; !ok {
		t.Error("pr-7's entry was taken away because the open pull requests could not be read")
	}
}

type pullsDown struct{ *fakeGitHub }

func (pullsDown) pulls() ([]pull, error) { return nil, errDown }

// A signed build goes up before its entry, so no app is offered a build
// it cannot download yet, #420, and a pull request's commit is marked the
// moment its entry is up, #839.
//
// On the real disk: the release tool reads the zip and the entry from files.
func TestAPublishedBuildIsMarkedOnceItsEntryIsUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	zip, entry := filepath.Join(dir, "app.zip"), filepath.Join(dir, "entry.json")
	for _, f := range []string{zip, entry} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gh := &fakeGitHub{}
	if err := publish(gh, zip, "app-pr-7-v.zip", entry, "pr-7", "c7"); err != nil {
		t.Fatal(err)
	}
	if err := publish(gh, zip, "app-main-v.zip", entry, "main", "cm"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"upload app-pr-7-v.zip", "upload channel-pr-7.json", "mark c7 success: On the Updates page as pr-7 ",
		// main's commits carry no mark: main is followed, not tested.
		"upload app-main-v.zip", "upload channel-main.json",
	}
	if !slices.Equal(gh.did, want) {
		t.Errorf("did\n  %s\nwant\n  %s", strings.Join(gh.did, "\n  "), strings.Join(want, "\n  "))
	}
}

// A build that is not published says why on its commit, whatever failed.
func TestABuildNotPublishedSaysWhy(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{}
	refusal := "the build could read input meant for other apps ...:\n  Frame Fairy.app/Contents/MacOS/framefairy-app: imports _CGWindowListCreateImage\n  other\n"
	for _, c := range []struct{ refusal, want string }{
		{refusal, "Not signed: Frame Fairy.app/Contents/MacOS/framefairy-app: imports _CGWindowListCreateImage"},
		{"FRAMEFAIRY_UPDATE_KEY is not set\n", "Not signed: FRAMEFAIRY_UPDATE_KEY is not set"},
		{"", "Not published: a step of its publishing failed, see the run"},
	} {
		gh.did = nil
		if err := notPublished(gh, "c", []byte(c.refusal), "https://run"); err != nil {
			t.Fatal(err)
		}
		if want := "mark c failure: " + c.want + " https://run"; len(gh.did) != 1 || gh.did[0] != want {
			t.Errorf("marked %q, want %q", gh.did, want)
		}
	}
}

// A publish that waits half an hour to enter its environment is
// cancelled, and one that waits less is left alone.
func TestAPublishStuckBeforeItsEnvironmentIsCancelled(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{stuck: []waitingRun{
		{id: 1, created: now.Add(-31 * time.Minute)},
		{id: 2, created: now.Add(-29 * time.Minute)},
	}}
	if err := unstick(gh, now, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gh.did, []string{"cancel 1"}) {
		t.Errorf("did %v, want only run 1 cancelled", gh.did)
	}
}
