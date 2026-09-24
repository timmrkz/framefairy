package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// appMenu is the menu Wails gives every app, with two changes: Undo and
// Redo in Edit belong to the app, and so does Help.
//
// The stock ones hand the keys to the web view, whose undo only knows about
// text being typed. On macOS the menu takes Cmd-Z before the page ever sees
// it, so without this the key would never reach an edit of a clip at all.
// These send the interface an event, and the interface decides: the text of
// a field being typed in when there is one, the episode's last edit
// otherwise.
func appMenu(app *application.App) *application.Menu {
	menu := application.NewMenu()
	if runtime.GOOS != "windows" {
		menu.AddRole(application.AppMenu)
	}
	menu.AddRole(application.FileMenu)
	edit := menu.AddSubmenu("Edit")
	edit.Add("Undo").SetAccelerator("CmdOrCtrl+z").OnClick(func(*application.Context) {
		app.Event.Emit("undo", "undo")
	})
	// Redo is Cmd-Y, the key most people know it by, and Ctrl-Y on
	// Windows and Linux. Shift-Cmd-Z is not bound to anything.
	edit.Add("Redo").SetAccelerator("CmdOrCtrl+y").OnClick(func(*application.Context) {
		app.Event.Emit("undo", "redo")
	})
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	if runtime.GOOS == "darwin" {
		edit.AddRole(application.PasteAndMatchStyle)
	}
	edit.AddRole(application.Delete)
	edit.AddRole(application.SelectAll)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	// Help holds what apps keep out of the way and within reach, the
	// notices of the work Frame Fairy is made with. The stock Help menu
	// held one item, Learn More, which opened wails.io in the app's own
	// window.
	help := menu.AddSubmenu("Help")
	help.Add("Acknowledgements").OnClick(func(*application.Context) {
		app.Event.Emit("acknowledgements", nil)
	})
	return menu
}
