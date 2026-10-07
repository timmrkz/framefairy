//go:build !outside

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// Every build but the one the checks from outside the app use has no
// probe, see probe_outside.go.

func probeMiddleware(next application.Middleware) application.Middleware { return next }

func startProbe(*FrameFairy) {}

func probeLink() {}
