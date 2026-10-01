package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"framefairy/engine"
)

// ClipPlayed records that a clip was watched in the app.
func (s *FrameFairy) ClipPlayed(plan, clipID string) error {
	if !s.store.Known(plan) {
		return errNotInLibrary
	}
	return engine.RecordDecision(plan, clipID, engine.DecisionViewed, nil)
}

// TrainingStatus is the one folder the records live in and how much is in
// it, for the settings screen.
type TrainingStatus struct {
	Dir       string `json:"dir"`
	Plans     int    `json:"plans"`
	Decisions int    `json:"decisions"`
}

// Training says where the records are and how many there are. The folder
// belongs to the person, not to an episode, so it is named as it is on
// disk rather than hidden behind a setting.
func (s *FrameFairy) Training() TrainingStatus {
	dir := engine.TrainingDir()
	return TrainingStatus{
		Dir:       dir,
		Plans:     countRecords(filepath.Join(dir, "plans.jsonl")),
		Decisions: countRecords(filepath.Join(dir, "decisions.jsonl")),
	}
}

// countRecords counts the lines of a record file. A missing file is none.
func countRecords(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) > 0 {
			n++
		}
	}
	return n
}

// ClearTraining removes the records, all of them. What they were for is
// training a model that is not trained yet, so this is a person tidying up
// while the app is being built, and it cannot be taken back.
func (s *FrameFairy) ClearTraining() error {
	dir := engine.TrainingDir()
	for _, name := range []string{"plans.jsonl", "decisions.jsonl"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
