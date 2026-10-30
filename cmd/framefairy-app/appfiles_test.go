package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// Tim moves between builds every day on the Updates page, and a pull
// request built on an older main is an older build. What a newer build
// saved in the app's own files has to outlive the older one saving them,
// and a file the app cannot read is never written over with nothing. #612.

const appConfig = "/home/tim/config/FrameFairy"

// written reads a JSON object the app wrote back into its keys.
func written(t *testing.T, files afero.Fs, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := afero.ReadFile(files, path)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("%s is no object: %v\n%s", path, err, data)
	}
	// The settings are written indented, which is not what is compared.
	for k, v := range keys {
		var flat bytes.Buffer
		if err := json.Compact(&flat, v); err != nil {
			t.Fatal(err)
		}
		keys[k] = flat.Bytes()
	}
	return keys
}

func TestASettingANewerBuildSavedOutlivesASaveByAnOlderOne(t *testing.T) {
	files := afero.NewMemMapFs()
	path := filepath.Join(appConfig, "settings.json")
	newer := `{
  "LLMModel": "/models/old.gguf",
  "planner": "api",
  "chosen": true,
  "captionFont": {"name": "Inter", "weight": 700},
  "ffmpeg": "/usr/local/bin/ffmpeg",
  "llmServer": "/usr/local/bin/llama-server",
  "count": 12
}`
	if err := afero.WriteFile(files, path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	st := openStoreIn(files, appConfig)
	if err := st.UpdateSettings(func(s *Settings) {
		s.Target = 3
		s.LLMModel = "/models/new.gguf"
	}); err != nil {
		t.Fatal(err)
	}

	keys := written(t, files, path)
	if got := string(keys["captionFont"]); got != `{"name":"Inter","weight":700}` {
		t.Errorf("the setting the older build does not know came back as %q", got)
	}
	if got := string(keys["target"]); got != "3" {
		t.Errorf("the setting changed came back as %q", got)
	}
	// The key json read whatever its case is written once, as the field's,
	// or the old path would be read again over the new one.
	var models []string
	for k, v := range keys {
		if strings.EqualFold(k, "llmModel") {
			models = append(models, k+"="+string(v))
		}
	}
	if len(models) != 1 || models[0] != `llmModel="/models/new.gguf"` {
		t.Errorf("the model is written as %v, want only llmModel=\"/models/new.gguf\"", models)
	}
	// Retired on purpose: the paths of programs the app no longer runs.
	for _, gone := range []string{"ffmpeg", "llmServer", "count"} {
		if v, ok := keys[gone]; ok {
			t.Errorf("the retired %s was written back as %s", gone, v)
		}
	}
	if again := openStoreIn(files, appConfig).Settings(); again.LLMModel != "/models/new.gguf" || again.Planner != "api" || !again.Chosen {
		t.Errorf("the settings read back as %+v", again)
	}
}

func TestWhatANewerBuildKeptInAVideoOutlivesChoosingAClip(t *testing.T) {
	s, source := chosenApp(t)
	file := filepath.Join(engine.WorkDir(source), chosenFile)
	newer := `{"clip":"clips/abc123","window":{"from":0,"to":60,"length":60},"zoom":{"from":12.5,"to":20}}`
	if err := afero.WriteFile(s.store.files, file, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseClip(source, "clips/def456"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseWindow(source, 120, 180, 60, false); err != nil {
		t.Fatal(err)
	}
	keys := written(t, s.store.files, file)
	if got := string(keys["zoom"]); got != `{"from":12.5,"to":20}` {
		t.Errorf("what the older build does not know came back as %q", got)
	}
	if got := string(keys["clip"]); got != `"clips/def456"` {
		t.Errorf("the clip came back as %s", got)
	}
	if got := s.ChosenWindow(source); got == nil || *got != (KeptWindow{From: 120, To: 180, Length: 60}) {
		t.Errorf("the window came back as %+v", got)
	}
}

// A file of the app's own in a shape this build cannot read, from a later
// build or edited by hand, is set aside whole before the first save, and
// what the build could not read is not taken half: a library that is not
// a list used to start empty, and the next Add saved only the new video.
func TestAFileTheAppCannotReadIsSetAsideNotWrittenOver(t *testing.T) {
	cases := []struct {
		name, file, body string
		save             func(*store) error
	}{
		{"a library that is not a list", "library.json", `{"videos": ["/home/tim/videos/a.mp4", "/home/tim/videos/b.mp4"]}`,
			func(st *store) error {
				_, err := st.AddEpisodes([]string{"/home/tim/videos/c.mp4"})
				return err
			}},
		{"a library cut off halfway", "library.json", `["/home/tim/videos/a.mp4", "/home/ti`,
			func(st *store) error { return st.RemoveEpisode("/home/tim/videos/a.mp4") }},
		{"settings with a value of the wrong kind", "settings.json", `{"llmModel": "/models/m.gguf", "target": "five"}`,
			func(st *store) error { return st.UpdateSettings(func(s *Settings) { s.Max = 40 }) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := afero.NewMemMapFs()
			path := filepath.Join(appConfig, c.file)
			if err := afero.WriteFile(files, path, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			st := openStoreIn(files, appConfig)
			if got := st.Episodes(); len(got) != 0 {
				t.Errorf("the library opened with %v", got)
			}
			if got := st.Settings().LLMModel; got != "" {
				t.Errorf("settings that could not be read were taken in part, the model %q", got)
			}
			// Nothing is written until the store saves.
			if body, _ := afero.ReadFile(files, path); string(body) != c.body {
				t.Fatalf("opening the store changed %s to %s", c.file, body)
			}
			if err := c.save(st); err != nil {
				t.Fatal(err)
			}
			aside, err := afero.Glob(files, path+".unreadable-*")
			if err != nil || len(aside) != 1 {
				t.Fatalf("set aside as %v, %v, want one file", aside, err)
			}
			if body, _ := afero.ReadFile(files, aside[0]); string(body) != c.body {
				t.Errorf("what was set aside is %q, want what was there", body)
			}
			// The next save writes over the store's own file and sets
			// nothing more aside.
			if err := c.save(st); err != nil {
				t.Fatal(err)
			}
			if again, _ := afero.Glob(files, path+".unreadable-*"); len(again) != 1 {
				t.Errorf("a second save set aside %v", again)
			}
		})
	}
}

func TestAChosenFileTheAppCannotReadIsSetAside(t *testing.T) {
	s, source := chosenApp(t)
	file := filepath.Join(engine.WorkDir(source), chosenFile)
	newer := `{"clip": ["clips/abc", "clips/def"]}`
	if err := afero.WriteFile(s.store.files, file, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseClip(source, "clips/def456"); err != nil {
		t.Fatal(err)
	}
	aside, _ := afero.Glob(s.store.files, file+".unreadable-*")
	if len(aside) != 1 {
		t.Fatalf("set aside as %v, want one file", aside)
	}
	if body, _ := afero.ReadFile(s.store.files, aside[0]); string(body) != newer {
		t.Errorf("what was set aside is %q", body)
	}
	if got := s.ChosenClip(source); got != "clips/def456" {
		t.Errorf("the clip chosen came back as %q", got)
	}
}
