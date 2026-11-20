//go:build !darwin

package engine

import (
	"os"
	"path/filepath"
	"runtime"
)

// systemFontFolders adds the folders Windows or Linux keeps its fonts in,
// the user's own first, see FontFolders.
func systemFontFolders(home string, add func(string)) {
	if runtime.GOOS == "windows" {
		add(filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"))
		add(filepath.Join(os.Getenv("WINDIR"), "Fonts"))
		return
	}
	if home != "" {
		add(filepath.Join(home, ".local", "share", "fonts"))
		add(filepath.Join(home, ".fonts"))
	}
	add("/usr/local/share/fonts")
	add("/usr/share/fonts")
}
