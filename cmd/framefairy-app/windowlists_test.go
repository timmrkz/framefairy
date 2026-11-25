package main

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// Every list and map Go hands the window that api.ts reads as always
// there, filledWhereBuilt, is made where its type is built, empty and
// never nil, because encoding/json writes a nil one as null and the
// window that reads .length of null stops drawing. Each builder is asked
// for what it builds from as little as it is given: a video not heard
// yet, a clip with no keyframes, a video's news with no clip changed and
// with none at all, no report, nothing dropped and nothing said.
func TestEveryListTheWindowReadsIsMadeWhereItsTypeIsBuilt(t *testing.T) {
	t.Parallel()
	svc, mine := memoryLibrary(t, "ep.mp4", "unheard.mp4")
	video := mine[0]
	withJobs(svc)
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(historyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	heardEpisode(t, svc, video, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}, {11.0, 11.4, "drei"}})
	ctx := context.Background()
	var sent []any
	keep := func(v any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		sent = append(sent, v)
	}
	state, err := svc.State(ctx, video)
	keep(state, err)
	// A video's state as its news sends it: whole the first time, then
	// with no clip changed, and then with none left.
	news, removed := &telling{}, state
	removed.Clips = []ClipEntry{}
	sent = append(sent, news.trim(state), news.trim(state), news.trim(removed))
	keep(svc.Shape(video, plan, "01", engine.Gesture{Kind: "trim", Edge: "end", From: 14, To: 14}, ""))
	keep(svc.Captions(video, plan, "01"))
	keep(svc.Licences())
	keep(svc.StopListening())
	sent = append(sent, svc.framingOf(video, engine.SourceInfo{}, nil, nil, nil), svc.Library(), svc.Episode(mine[1]),
		svc.Setup(ctx), svc.ReportDraft(), svc.dropFiles(nil), svc.CaptionStart(),
		Dictation{Said: copySaid(nil)})

	seen, null := map[string]bool{}, map[string]bool{}
	for _, v := range sent {
		listsIn(reflect.ValueOf(v), seen, null)
	}
	for _, key := range slices.Sorted(func(yield func(string) bool) {
		for key := range filledWhereBuilt {
			if !yield(key) {
				return
			}
		}
	}) {
		switch {
		case !seen[key]:
			t.Errorf("%s was never built here, so nothing shows it is made", key)
		case null[key]:
			t.Errorf("%s is sent as null", key)
		}
	}
}

// listsIn walks v the way encoding/json writes it, and for every field of
// filledWhereBuilt it meets says it was seen, and whether it was nil.
func listsIn(v reflect.Value, seen, null map[string]bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			listsIn(v.Elem(), seen, null)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			listsIn(v.Index(i), seen, null)
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			listsIn(it.Value(), seen, null)
		}
	case reflect.Struct:
		fieldsIn(v, v.Type().String(), seen, null)
	}
}

// fieldsIn walks the fields of a struct, those of a struct it embeds as
// its own, the way encoding/json and the channel test name them.
func fieldsIn(v reflect.Value, owner string, seen, null map[string]bool) {
	for i := range v.NumField() {
		f := v.Type().Field(i)
		tag := f.Tag.Get("json")
		if !f.IsExported() || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		field := v.Field(i)
		if f.Anonymous && name == "" {
			inner := field
			for inner.Kind() == reflect.Pointer && !inner.IsNil() {
				inner = inner.Elem()
			}
			if inner.Kind() == reflect.Struct {
				fieldsIn(inner, owner, seen, null)
				continue
			}
		}
		if name == "" {
			name = f.Name
		}
		if key := owner + "." + name; filledWhereBuilt[key] {
			seen[key] = true
			if (field.Kind() == reflect.Slice || field.Kind() == reflect.Map || field.Kind() == reflect.Pointer) && field.IsNil() {
				null[key] = true
			}
		}
		listsIn(field, seen, null)
	}
}
