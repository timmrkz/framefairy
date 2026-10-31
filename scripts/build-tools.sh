#!/bin/sh
# Installs what builds the ffmpeg and the llama-server we ship, cmake,
# meson, ninja and pkg-config, at the versions requirements.txt pins with
# the hashes of their files, into a Python environment of their own.
#
#   sh scripts/build-tools.sh [DIR [PACKAGE...]]
#
# DIR is .build/tools by default, and the packages all four. The episode's
# decoder needs only pkg-config, so a job whose ffmpeg came from the cache
# asks for pkgconf alone and does not fetch cmake's 52 MB.
#
# The tools are then in DIR/bin, which the Mac jobs of CI and
# scripts/tools.sh put first on the path for the builds, and the Makefile
# for the episode's decoder, which finds ffmpeg's libraries with
# pkg-config. They came from Homebrew, which installs only its newest
# version, so what built ffmpeg changed with the day it was built, #591.
#
# It does nothing when DIR holds the tools of the requirements.txt there
# is now, so make can ask it every time a build needs them.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
dir=${1:-.build/tools}
[ $# -gt 0 ] && shift
want=" ${*:-cmake meson ninja pkgconf} "
pins=$here/../requirements.txt

# The tools' own entries, each with its hashes: pip takes a requirements
# file whole, and the speech benchmark's pyarrow is no tool of the build.
list=$(awk -v want="$want" '
	/^[A-Za-z0-9_.-]+==/ {
		split($1, n, "==")
		take = index(want, " " n[1] " ") > 0
	}
	/^[[:space:]]*(#|$)/ { take = 0; next }
	take { print }
' "$pins")
recipe=$(printf '%s\n' "$list" | cksum | cut -d' ' -f1)
if [ -z "$list" ]; then
	echo "build-tools.sh: requirements.txt pins none of$want" >&2
	exit 1
fi
if [ -x "$dir/bin/python" ] && [ "$(cat "$dir/built" 2>/dev/null)" = "$recipe" ]; then
	exit 0
fi

rm -rf "$dir"
python3 -m venv "$dir"
printf '%s\n' "$list" >"$dir/requirements.txt"
"$dir/bin/python" -m pip install --quiet --disable-pip-version-check --require-hashes --only-binary=:all: -r "$dir/requirements.txt"
echo "$recipe" >"$dir/built"
