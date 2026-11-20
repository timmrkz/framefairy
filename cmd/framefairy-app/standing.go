package main

import "strings"

// How a choice of model stands at rest: whether it can do its work now,
// and if not, what is in the way. Go judges it, from the settings, the
// keys, the models folder and whether llama-server is here, and the
// settings only word it, see ModelCard.svelte and lib/finding.ts. They
// judged it themselves once, a copy of the rules Setup already had for
// Ready, and the two could disagree, #610.
//
// Work in hand is not at rest: a download on its way, one that stopped,
// a search that uses the model. The settings show that from the job, the
// way every place that shows work does.

// Standing is how one choice of model stands, as if it were the one
// chosen, so the settings show a choice made by a click in the frame of
// the click, before Go has heard of it.
type Standing struct {
	// State is "ok" when the model can do its work now, "warn" when a
	// step is still to take, a key or a download, and "err" when what is
	// there does not work.
	State string `json:"state"`
	// Says is what the line under the choice says, in the settings' own
	// words: "cloud", a model in the cloud by its company, "fetch", one
	// not downloaded yet, "ready", one here, "folder", a speech model in a
	// folder named by hand, which Detail names, and "problem", what a
	// check found wrong, which Detail says.
	Says   string `json:"says"`
	Detail string `json:"detail,omitempty"`
}

// The words of Standing.State and Standing.Says.
const (
	standingOK   = "ok"
	standingWarn = "warn"
	standingErr  = "err"

	saysCloud   = "cloud"
	saysFetch   = "fetch"
	saysReady   = "ready"
	saysFolder  = "folder"
	saysProblem = "problem"
)

// cloudStanding is how a model in the cloud stands: ready once its
// company's key is here, a step away until it is.
func cloudStanding(hasKey bool) Standing {
	if hasKey {
		return Standing{State: standingOK, Says: saysCloud}
	}
	return Standing{State: standingWarn, Says: saysCloud}
}

// hereStanding is how a model that runs on this machine stands, as the
// one chosen. check is the language model's check, see languageCheck,
// and counts only for the model it was made for, the one in use: a model
// chosen by a click is judged by what is on disk until the check is made
// for it. server says whether llama-server is here, without which a model
// answers nothing.
func hereStanding(installed, inUse bool, check Check, server bool) Standing {
	if !installed {
		return Standing{State: standingWarn, Says: saysFetch}
	}
	if inUse && !check.OK {
		return Standing{State: standingErr, Says: saysProblem, Detail: check.Detail}
	}
	if !server {
		return Standing{State: standingErr, Says: saysReady}
	}
	return Standing{State: standingOK, Says: saysReady}
}

// speechStanding is how the speech model in use stands. check is the
// speech model's check, see speechCheck, named says its folder was named
// by hand in the settings, and installed that the model in use is in the
// models folder. A problem is the check's first line, so the line under
// the choice stays one line.
func speechStanding(check Check, named, installed bool) Standing {
	switch {
	case !check.OK:
		return Standing{State: standingErr, Says: saysProblem, Detail: firstLine(check.Detail)}
	case named:
		return Standing{State: standingOK, Says: saysFolder, Detail: check.Detail}
	case !installed:
		return Standing{State: standingWarn, Says: saysFetch}
	}
	return Standing{State: standingOK, Says: saysReady}
}

// firstLine is text up to its first line break.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
