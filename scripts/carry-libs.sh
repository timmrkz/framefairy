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

# --verify reads a program back without touching it, which is how the
# bundle is checked as well. Everything below is the same job either way.
ONLY_VERIFY=0
if [ "$1" = --verify ]; then
	ONLY_VERIFY=1
	shift
fi

PROGRAM="$1"
LIBDIR="$2"
if [ -z "$PROGRAM" ] || { [ "$ONLY_VERIFY" = 0 ] && [ -z "$LIBDIR" ]; }; then
	echo "usage: carry-libs.sh PROGRAM LIBDIR" >&2
	echo "       carry-libs.sh --verify PROGRAM" >&2
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

# Read back, rather than hoped. Every rewrite above is allowed to fail
# quietly, because most of them are no-ops, and a run where they all failed
# looks exactly like a run where none of them was needed.
#
# What was here before asked otool -L whether the module cache was still
# named, and the answer was always no, because on macOS nothing records an
# absolute path: the libraries call themselves @rpath/something and the
# program records that. The module cache was in LC_RPATH, which otool -L
# does not print. So the check passed while the thing it was written to
# catch was fully present, on every macOS build there has ever been.
#
# So it asks the question that decides whether the program runs on somebody
# else's machine: does each library it needs actually resolve, under the
# paths it is willing to look in, and is one of those paths a folder only
# this machine has.
verify() {
	program="$1"
	dir=$(cd "$(dirname "$program")" && pwd)
	bad=0
	case "$SYSTEM" in
	Darwin)
		# LC_RPATH, one per load command, which is where @rpath is allowed
		# to mean. And the libraries named @rpath/something, which are the
		# ones that have to be found under one of them.
		rpaths=$(otool -l "$program" 2>/dev/null |
			awk '/cmd LC_RPATH/{want=1} want && /^ *path /{print $2; want=0}')
		needs=$(otool -L "$program" 2>/dev/null | tail -n +2 |
			awk '{print $1}' | grep '^@rpath/' || true)
		for need in $needs; do
			base=${need#@rpath/}
			found=""
			for rp in $rpaths; do
				case "$rp" in
				@executable_path*) where="$dir${rp#@executable_path}" ;;
				@loader_path*) where="$dir${rp#@loader_path}" ;;
				*) where="$rp" ;;
				esac
				if [ -f "$where/$base" ]; then
					found="$where/$base"
					break
				fi
			done
			if [ -z "$found" ]; then
				echo "carry-libs.sh: $program needs $base and cannot find it." >&2
				bad=1
			fi
		done
		if [ -n "$GOMOD" ] && printf '%s\n' "$rpaths" | grep -qF "$GOMOD"; then
			echo "carry-libs.sh: $program still looks in the build machine's module" >&2
			echo "cache, which exists on no other machine:" >&2
			printf '%s\n' "$rpaths" | grep -F "$GOMOD" | sed 's/^/  /' >&2
			bad=1
		fi
		;;
	Linux)
		# ldd resolves $ORIGIN against the real program, so this is the
		# same question asked of the loader itself.
		if ldd "$program" 2>/dev/null | grep -q 'not found'; then
			echo "carry-libs.sh: $program needs libraries it cannot find:" >&2
			ldd "$program" 2>/dev/null | grep 'not found' | sed 's/^/  /' >&2
			bad=1
		fi
		if [ -n "$GOMOD" ] && readelf -d "$program" 2>/dev/null |
			grep -E 'RPATH|RUNPATH' | grep -qF "$GOMOD"; then
			echo "carry-libs.sh: $program still looks in the build machine's module" >&2
			echo "cache, which exists on no other machine." >&2
			bad=1
		fi
		;;
	esac
	if [ "$bad" != 0 ]; then
		echo "  It would run here and nowhere else. See docs/PACKAGING.md." >&2
		return 1
	fi
	return 0
}

if [ "$ONLY_VERIFY" = 1 ]; then
	verify "$PROGRAM"
	exit $?
fi

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
	# Where @rpath means. Relative to the program, so it can be moved
	# anywhere. Three shapes: beside the program, in a folder beside it,
	# which is what bin/lib is, and Contents/Frameworks for a bundle.
	#
	# The middle one was missing, and it is the one the everyday build
	# uses. Nothing noticed, for the reason just below.
	here=$(cd "$(dirname "$PROGRAM")" && pwd)
	there=$(cd "$LIBDIR" && pwd)
	install_name_tool -add_rpath "@executable_path" "$PROGRAM" 2>/dev/null || true
	install_name_tool -add_rpath "@executable_path/../Frameworks" "$PROGRAM" 2>/dev/null || true
	if [ "$there" != "$here" ]; then
		install_name_tool -add_rpath "@executable_path/$(basename "$there")" \
			"$PROGRAM" 2>/dev/null || true
	fi
	# And the one that was keeping the mistake invisible. cgo links this
	# program with -Wl,-rpath pointing at the module cache folder the
	# libraries were built into, so whatever else is wrong, the program
	# finds them there and runs perfectly on the machine that built it. It
	# is the only machine where that folder exists.
	#
	# Every install_name_tool -change above is a no-op on macOS, because
	# these libraries already call themselves @rpath/something and so does
	# everything that links them. Nothing records an absolute path at all.
	# The whole question is the rpath, so the module cache comes out of it.
	install_name_tool -delete_rpath "$FROM" "$PROGRAM" 2>/dev/null || true
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


verify "$PROGRAM"
