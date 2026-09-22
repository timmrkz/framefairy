#!/bin/sh
# Puts the built programs into "Frame Fairy.app", the folder Finder draws as
# one icon.
#
#   scripts/bundle-macos.sh [BINDIR] [OUTDIR]
#
# This is not only a packaging step. It is what make run starts, so the
# thing run every day and the thing a customer runs are the same shape: the
# same Info.plist, the same privacy prompts, the same ffmpeg and
# llama-server inside the same folder, the same speech libraries found the
# same way. The only thing a customer will have that this does not is the
# signature. A bug in any of the rest then shows up on the day it is made
# rather than on the day of a release.
#
# See docs/PACKAGING.md. Nothing here signs anything: the ad hoc signature
# carry-libs.sh leaves is enough to run on the machine that built it, and
# Developer ID signing and notarisation are their own step.
set -e

BINDIR=${1:-bin}
OUTDIR=${2:-bin}
NAME="Frame Fairy"
EXE=framefairy-app

if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "bundle-macos.sh: a .app is a macOS thing and this is not a Mac." >&2
	exit 1
fi
if [ ! -x "$BINDIR/$EXE" ]; then
	echo "bundle-macos.sh: $BINDIR/$EXE is not built. Run make first." >&2
	exit 1
fi

# The version lives in one place and is read from it, so a bundle can never
# claim a version the program does not.
ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION=$(awk -F'"' '/^const Version = /{ print $2; exit }' "$ROOT/engine/log.go")
[ -n "$VERSION" ] || {
	echo "bundle-macos.sh: cannot read the version out of engine/log.go" >&2
	exit 1
}

APP="$OUTDIR/$NAME.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Frameworks" "$APP/Contents/Resources"

cp "$BINDIR/$EXE" "$APP/Contents/MacOS/$EXE"

# The tools we ship, beside the program, which is where engine/tools.go
# looks before the search path. So the bundle renders with our ffmpeg and
# runs a local model with our llama-server, with no search path at all.
for tool in ffmpeg ffprobe llama-server; do
	if [ -x "$BINDIR/$tool" ]; then
		cp "$BINDIR/$tool" "$APP/Contents/MacOS/$tool"
	else
		echo "  no $tool in $BINDIR, so the bundle will look on the search path for it"
	fi
done
# Their licence texts travel with them. LGPL asks for it and MIT asks for
# it, and it is two files.
for licence in "$BINDIR"/LICENSE-*; do
	[ -f "$licence" ] && cp "$licence" "$APP/Contents/MacOS/"
done

# The speech libraries. carry-libs.sh has already rewritten the program to
# look in @executable_path/../Frameworks, which from Contents/MacOS is
# exactly this folder, so copying them here is the whole of it.
if [ -d "$BINDIR/lib" ]; then
	cp "$BINDIR"/lib/*.dylib "$APP/Contents/Frameworks/" 2>/dev/null || true
fi

# The icon, built from build/icon.png by sips and iconutil. See
# scripts/make-icon.sh, which also explains why that PNG is inset rather
# than filling its square.
ICON=""
ICNS="$ROOT/.build/icon.icns"
if [ -f "$ROOT/build/icon.png" ]; then
	sh "$ROOT/scripts/make-icon.sh" "$ICNS"
	cp "$ICNS" "$APP/Contents/Resources/icon.icns"
	ICON='	<key>CFBundleIconFile</key>
	<string>icon</string>'
else
	echo "  no build/icon.png, so this one wears the blank system icon"
fi

# The purpose strings are the words macOS puts in the prompt when the app
# reads a folder it has not been let into. An app with none of them gets a
# prompt with a blank reason, which is what somebody who has just paid for
# it would see. They say what the app does with the folder, because that is
# the question being asked.
cat >"$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>$NAME</string>
	<key>CFBundleDisplayName</key>
	<string>$NAME</string>
	<key>CFBundleExecutable</key>
	<string>$EXE</string>
	<key>CFBundleIdentifier</key>
	<string>com.framefairy.app</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
$ICON
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.video</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
	<key>NSDesktopFolderUsageDescription</key>
	<string>Frame Fairy opens the episodes you keep on the Desktop and writes each one's clips in a folder beside the video.</string>
	<key>NSDocumentsFolderUsageDescription</key>
	<string>Frame Fairy opens the episodes you keep in Documents and writes each one's clips in a folder beside the video.</string>
	<key>NSDownloadsFolderUsageDescription</key>
	<string>Frame Fairy opens the episodes you keep in Downloads and writes each one's clips in a folder beside the video.</string>
	<key>NSRemovableVolumesUsageDescription</key>
	<string>Frame Fairy opens the episodes you keep on an external disk and writes each one's clips in a folder beside the video.</string>
</dict>
</plist>
PLIST

# Four bytes that say the same as CFBundlePackageType. Old, still read by
# some of Finder, and it costs a line.
printf 'APPL????' >"$APP/Contents/PkgInfo"

# Rewriting nothing invalidates nothing, but the copy loses the signature
# on some systems, so it is made again. Ad hoc, the same as carry-libs.sh:
# enough to run here, and Developer ID signing is its own step later.
codesign --remove-signature "$APP/Contents/MacOS/$EXE" 2>/dev/null || true
codesign -s - -f "$APP/Contents/MacOS/$EXE" 2>/dev/null || true

# And read back, because everything above is a copy that is allowed to fail
# quietly and a bundle that is wrong looks exactly like one that is right
# until somebody else opens it.
#
# The question that decides whether this runs on a customer's Mac: does
# every speech library it needs resolve inside the bundle, and does it
# still look in the Go module cache of the machine that built it. That
# folder exists here and on no other Mac. carry-libs.sh asks it, so it is
# asked with the same eyes rather than a second pair.
sh "$(dirname "$0")/carry-libs.sh" --verify "$APP/Contents/MacOS/$EXE"

echo "$APP"
echo "  version $VERSION, $(du -sh "$APP" | cut -f1)"
echo "  in Contents/MacOS:     $(ls "$APP/Contents/MacOS" | tr '\n' ' ')"
echo "  in Contents/Frameworks: $(ls "$APP/Contents/Frameworks" | tr '\n' ' ')"
