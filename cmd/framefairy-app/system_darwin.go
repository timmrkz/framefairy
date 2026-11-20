package main

// What the app does differently on a Mac. Each has its counterpart for
// the other systems in system_other.go, and a pull request that changes
// one runs the Go tests on a Mac, see internal/maccode.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// systemLogPath is where a Mac app keeps its log, in the user's
// Library, see appLogPath.
func systemLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", "Frame Fairy", "app.log")
}

// showCommand shows a file in Finder, see showInFolder.
func showCommand(path string) *exec.Cmd {
	return exec.Command("open", "-R", path)
}

// heardNothing is what a recording of nothing but zeros says. macOS
// hands over zeros when the app is not allowed the microphone.
func heardNothing() string {
	return "the microphone heard nothing. If Frame Fairy may not use it, allow it in System Settings, Privacy & Security, Microphone."
}

// addAppMenu adds the app menu, the stock one with Check for Updates in
// it, where every Mac app keeps it. The first menu is the app menu
// whatever it is called.
func addAppMenu(menu *application.Menu, checkForUpdates func()) {
	own := menu.AddSubmenu("Frame Fairy")
	own.AddRole(application.About)
	own.Add("Check for Updates…").OnClick(func(*application.Context) { checkForUpdates() })
	own.AddSeparator()
	own.AddRole(application.ServicesMenu)
	own.AddSeparator()
	own.AddRole(application.Hide)
	own.AddRole(application.HideOthers)
	own.AddRole(application.UnHide)
	own.AddSeparator()
	own.AddRole(application.Quit)
}

// addPasteAndMatchStyle adds the Edit menu's Paste and Match Style,
// which only a Mac has.
func addPasteAndMatchStyle(edit *application.Menu) {
	edit.AddRole(application.PasteAndMatchStyle)
}

// systemAbout is the version of macOS and the chip, for a report.
func systemAbout() (version, chip string) {
	return asked("sw_vers", "-productVersion"), asked("sysctl", "-n", "machdep.cpu.brand_string")
}

// asked runs a short command for a report and gives what it said, or
// nothing.
func asked(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
