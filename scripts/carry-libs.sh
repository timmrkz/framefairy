#!/bin/sh
# Carries the speech library next to a program and points the program at it.
#
#   carry-libs.sh PROGRAM LIBDIR
#
# sherpa-onnx ships shared libraries rather than an archive, and its cgo
# directive writes the Go module cache path into the program as the place to
# find them:
#
#   RUNPATH /Users/tim/go/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-macos@v1.13.8/lib/...
#
# Nobody else has that folder, so the program dies before it draws anything.
# It has gone unnoticed because everyone who has run the app also built it.
# See docs/PACKAGING.md.
#
# This copies the libraries to LIBDIR and rewrites the program to look
# beside itself instead. It is the same job on each system and three
# different tools, so it is one script rather than three.
set -e

PROGRAM="$1"
LIBDIR="$2"
if [ -z "$PROGRAM" ] || [ -z "$LIBDIR" ]; then
	echo "usage: carry-libs.sh PROGRAM LIBDIR" >&2
	exit 2
fi
if [ ! -f "$PROGRAM" ]; then
	echo "carry-libs.sh: no such program: $PROGRAM" >&2
	exit 1
fi

SYSTEM=$(uname -s 2>/dev/null)
GOMOD=$(go env GOMODCACHE)
VERSION=$(awk '/k2-fsa\/sherpa-onnx-go-/ { print $2; exit }' "$(dirname "$0")/../go.mod")
if [ -z "$VERSION" ]; then
	VERSION=$(awk '/k2-fsa\/sherpa-onnx-go /{ print $2; exit }' "$(dirname "$0")/../go.mod")
fi

# Where the prebuilt libraries for this system live inside the module cache.
case "$SYSTEM" in
Darwin)
	case "$(uname -m)" in
	arm64) TRIPLE=aarch64-apple-darwin ;;
	*) TRIPLE=x86_64-apple-darwin ;;
	esac
	FROM="$GOMOD/github.com/k2-fsa/sherpa-onnx-go-macos@$VERSION/lib/$TRIPLE"
	;;
Linux)
	case "$(uname -m)" in
	aarch64 | arm64) TRIPLE=aarch64-unknown-linux-gnu ;;
	*) TRIPLE=x86_64-unknown-linux-gnu ;;
	esac
	FROM="$GOMOD/github.com/k2-fsa/sherpa-onnx-go-linux@$VERSION/lib/$TRIPLE"
	;;
*)
	# Windows keeps the DLLs beside the program and needs no rewriting,
	# which check.sh already does.
	sh "$(dirname "$0")/check.sh" --copy-dlls "$LIBDIR"
	exit 0
	;;
esac

if [ ! -d "$FROM" ]; then
	echo "carry-libs.sh: no speech library at $FROM" >&2
	echo "  run make once so the module is downloaded" >&2
	exit 1
fi

mkdir -p "$LIBDIR"
cp "$FROM"/* "$LIBDIR"/
chmod u+w "$LIBDIR"/*

case "$SYSTEM" in
Darwin)
	# Two halves on macOS. Each library says what it calls itself, and that
	# name is what anything linking it records, so the name has to be made
	# relative first. Then the program is told where to look.
	for lib in "$LIBDIR"/*.dylib; do
		base=$(basename "$lib")
		install_name_tool -id "@rpath/$base" "$lib" 2>/dev/null || true
		# A library that calls another one records the same absolute path.
		for other in "$LIBDIR"/*.dylib; do
			otherbase=$(basename "$other")
			install_name_tool -change "$FROM/$otherbase" "@rpath/$otherbase" "$lib" 2>/dev/null || true
		done
	done
	for other in "$LIBDIR"/*.dylib; do
		otherbase=$(basename "$other")
		install_name_tool -change "$FROM/$otherbase" "@rpath/$otherbase" "$PROGRAM" 2>/dev/null || true
	done
	# Where @rpath means. Relative to the program, so the bundle can be
	# dragged anywhere. Both shapes: beside the program for a plain build,
	# and Contents/Frameworks for a bundle.
	install_name_tool -add_rpath "@executable_path" "$PROGRAM" 2>/dev/null || true
	install_name_tool -add_rpath "@executable_path/../Frameworks" "$PROGRAM" 2>/dev/null || true
	# The signature is invalidated by every rewrite above, so it is made
	# again. Ad hoc here, because the real Developer ID signing happens
	# later over the whole bundle.
	codesign --remove-signature "$PROGRAM" 2>/dev/null || true
	codesign -s - -f "$PROGRAM" 2>/dev/null || true
	for lib in "$LIBDIR"/*.dylib; do
		codesign --remove-signature "$lib" 2>/dev/null || true
		codesign -s - -f "$lib" 2>/dev/null || true
	done
	;;
Linux)
	# One step on Linux. $ORIGIN is the folder the program is in, read at
	# load time, so it survives being moved.
	if ! command -v patchelf >/dev/null 2>&1; then
		echo "carry-libs.sh: patchelf is needed to point the program at its libraries" >&2
		echo "  sudo apt install patchelf" >&2
		exit 1
	fi
	here=$(cd "$(dirname "$PROGRAM")" && pwd)
	there=$(cd "$LIBDIR" && pwd)
	case "$there" in
	"$here") RPATH='$ORIGIN' ;;
	*) RPATH='$ORIGIN/'$(basename "$there") ;;
	esac
	patchelf --set-rpath "$RPATH" "$PROGRAM"
	;;
esac
