package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// The app's own files, the settings, the library and the chosen.json of
// each video, are read by every build Tim moves between on the Updates
// page, an older one as well as a newer one. A struct read from such a
// file and written back whole drops every key the build does not know, so
// an older build erased what a newer one had saved the first time it
// saved. So a file is read into its struct with the keys the struct does
// not name kept beside it, and they are written back as they were. A
// file that cannot be read at all is set aside before it is written over,
// never replaced by an empty one. #612.

// unknownKeys are the keys of a JSON object that a struct does not name,
// with their values as they were written.
type unknownKeys map[string]json.RawMessage

// readKeeping reads data into the value into points to, and gives back
// the keys of the object it names nowhere. On an error into is left as it
// was, not half filled: json fills in what it can before it says a value
// was of the wrong type. A file that holds no object, the library's list,
// keeps no keys.
func readKeeping(data []byte, into any) (unknownKeys, error) {
	was := reflect.ValueOf(into).Elem()
	fresh := reflect.New(was.Type())
	fresh.Elem().Set(was)
	if err := json.Unmarshal(data, fresh.Interface()); err != nil {
		return nil, err
	}
	was.Set(fresh.Elem())
	var all unknownKeys
	if json.Unmarshal(data, &all) != nil {
		return nil, nil
	}
	// json matches a key to a field whatever its case, so a key it read
	// into the struct is not kept beside it, or it would be read again
	// over the field's newer value the next time.
	names := jsonNames(was.Type())
	for key := range all {
		for _, n := range names {
			if strings.EqualFold(key, n) {
				delete(all, key)
				break
			}
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

// jsonNames are the keys json reads into a struct's fields.
func jsonNames(t reflect.Type) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case name == "-":
			continue
		case name == "" && f.Anonymous:
			names = append(names, jsonNames(f.Type)...)
			continue
		case name == "":
			name = f.Name
		}
		names = append(names, name)
	}
	return names
}

// writeKeeping is value as JSON, with the keys kept from the file put back
// beside its own.
func writeKeeping(value any, kept unknownKeys) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil || len(kept) == 0 {
		return body, err
	}
	var all unknownKeys
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, fmt.Errorf("only an object keeps keys: %w", err)
	}
	for k, v := range kept {
		if _, own := all[k]; !own {
			all[k] = v
		}
	}
	return json.Marshal(all)
}

// setAside moves a file the app could not read out of the way of the one
// about to be written, beside it, under a name that says so and when, and
// says in the log where it went. What is in it is never lost: a later
// build may read it, and a person can.
func setAside(files afero.Fs, path, why string, now time.Time) {
	aside := path + ".unreadable-" + now.UTC().Format("20060102-150405")
	ev := theLog.line(zerolog.ErrorLevel, "library").Str("file", filepath.Base(path)).Str("why", why)
	if err := files.Rename(path, aside); err != nil {
		ev.Err(err).Msg("a file the app could not read could not be set aside, so it is written over")
		return
	}
	ev.Str("aside", filepath.Base(aside)).Msg("a file the app could not read is set aside before it is written again")
}
