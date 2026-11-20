package engine

import "path/filepath"

// systemFontFolders adds the folders a Mac keeps its fonts in, the
// user's own first, see FontFolders.
func systemFontFolders(home string, add func(string)) {
	if home != "" {
		add(filepath.Join(home, "Library", "Fonts"))
	}
	add("/Library/Fonts")
	add("/System/Library/Fonts")
	// The fonts Font Book downloads when one is asked for, which the
	// system keeps among its assets, one folder for each version of the
	// catalogue.
	assets, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*")
	for _, dir := range assets {
		add(dir)
	}
}
