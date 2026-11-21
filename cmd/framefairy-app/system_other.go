//go:build !darwin

package main

// What the app does on Windows and Linux where a Mac does something of
// its own, in system_darwin.go.

import (
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/afero"
	"github.com/wailsapp/wails/v3/pkg/application"

	"framefairy/engine/fileio"
)

// systemLogPath is empty: the log goes to the user's cache folder, see
// appLogPath.
func systemLogPath() string { return "" }

// showCommand shows a file in Explorer, or its folder in the file
// manager, see showInFolder.
func showCommand(path string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// One argument, not two: with a space after the comma Explorer
		// opens its default folder and selects nothing.
		return exec.Command("explorer", "/select,"+path)
	}
	dir := path
	if fileio.IsFile(afero.NewOsFs(), path) {
		dir = filepath.Dir(path)
	}
	return exec.Command("xdg-open", dir)
}

// heardNothing is what a recording of nothing but zeros says.
func heardNothing() string {
	return "the microphone heard nothing. Check that one is connected and that Frame Fairy may use it."
}

// addAppMenu adds the stock app menu on Linux. Windows has none.
func addAppMenu(menu *application.Menu, _ func()) {
	if runtime.GOOS != "windows" {
		menu.AddRole(application.AppMenu)
	}
}

// addPasteAndMatchStyle adds nothing: only a Mac has it.
func addPasteAndMatchStyle(*application.Menu) {}

// systemAbout says nothing more of the system than its name, for a
// report.
func systemAbout() (version, chip string) { return "", "" }
