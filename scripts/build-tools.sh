#!/bin/sh
# Installs what builds the ffmpeg and the llama-server we ship, cmake,
# meson, ninja and pkg-config, at the versions requirements.txt pins with
# the hashes of their files, into a Python environment of their own.
#
#   sh scripts/build-tools.sh [DIR [PACKAGE...]]
#   sh scripts/build-tools.sh --recipe
#   sh scripts/build-tools.sh --then COMMAND...
#
# DIR is .build/tools by default, and the packages the four tools. Any
# other package requirements.txt pins can be asked for by name: the
# episode's decoder needs only pkg-config, so a CI job whose ffmpeg came
# from the cache asks for pkgconf alone and does not fetch cmake's 52 MB,
# and the speech benchmark asks for pyarrow.
#
# --recipe prints a checksum of the four tools' pins, which the caches of
# CI's Mac builds are keyed on, so a newer tool builds them again.
#
# --then runs COMMAND with the four tools first on the path, installed in
# .build/tools first. That is how scripts/tools.sh, make ffmpeg and make
# llama build on a Mac. Elsewhere it runs COMMAND as it is, with the
# system's tools, which the CI image takes from a dated snapshot.
#
# The tools came from Homebrew, which installs only its newest version,
# so what built ffmpeg changed with the day it was built, #591. It does
# nothing when DIR holds what requirements.txt pins now, so make can ask
# it every time a build needs the tools.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
pins=$here/../requirements.txt
tools="cmake meson ninja pkgconf"

# The entries of the packages asked for, each with its hashes: pip takes
# a requirements file whole.
entries() {
	awk -v want=" $* " '
		/^[A-Za-z0-9_.-]+==/ {
			split($1, n, "==")
			take = index(want, " " n[1] " ") > 0
		}
		/^[[:space:]]*(#|$)/ { take = 0; next }
		take { print }
	' "$pins"
}

case ${1:-} in
--recipe)
	entries $tools | cksum | cut -d' ' -f1
	exit 0
	;;
--then)
	shift
	if [ "$(uname -s)" = Darwin ]; then
		sh "$0" "$here/../.build/tools"
		PATH="$here/../.build/tools/bin:$PATH"
		export PATH
	fi
	exec "$@"
	;;
esac

dir=${1:-.build/tools}
[ $# -gt 0 ] && shift
want=${*:-$tools}
list=$(entries $want)
if [ -z "$list" ]; then
	echo "build-tools.sh: requirements.txt pins none of $want" >&2
	exit 1
fi
recipe=$(printf '%s\n' "$list" | cksum | cut -d' ' -f1)
if [ -x "$dir/bin/python" ] && [ "$(cat "$dir/built" 2>/dev/null)" = "$recipe" ]; then
	exit 0
fi

# meson and pkgconf need a Python of 3.10 or newer. The one macOS brings
# is 3.9, so Homebrew's goes first where there is one.
python=""
for p in /opt/homebrew/bin/python3 /usr/local/bin/python3 python3; do
	if "$p" -c 'import sys; sys.exit(sys.version_info < (3, 10))' 2>/dev/null; then
		python=$p
		break
	fi
done
if [ -z "$python" ]; then
	echo "build-tools.sh: the tools need a Python of 3.10 or newer. On macOS: make tools." >&2
	exit 1
fi
rm -rf "$dir"
"$python" -m venv "$dir"
printf '%s\n' "$list" >"$dir/requirements.txt"
"$dir/bin/python" -m pip install --quiet --disable-pip-version-check --require-hashes --only-binary=:all: -r "$dir/requirements.txt"
echo "$recipe" >"$dir/built"
