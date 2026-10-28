// Package application stands in for Wails' own, with only the names the
// rules on input are about.
package application

type App struct {
	Clipboard      *ClipboardManager
	GlobalShortcut *GlobalShortcutManager
}

type ClipboardManager struct{}

func (*ClipboardManager) Text() (string, bool) { return "", false }
func (*ClipboardManager) SetText(string) bool  { return true }

type GlobalShortcutManager struct{}

func (*GlobalShortcutManager) Register(string, func()) error { return nil }
