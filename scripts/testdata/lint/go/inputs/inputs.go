package inputs

import "github.com/wailsapp/wails/v3/pkg/application"

// What hears keys pressed in other apps, or reads what was copied in
// them, however it is reached.
func listen(app *application.App) {
	_ = app.GlobalShortcut.Register("F1", func() {})
	_, _ = app.Clipboard.Text()
	c := app.Clipboard
	_, _ = c.Text()
	// Putting something on the clipboard reads nothing.
	app.Clipboard.SetText("link")
}
