#!/bin/sh
# Everything this machine needs to build and run framefairy, installed
# where it is missing.
#
#   scripts/tools.sh [FFMPEG_DIR] [LLAMA_DIR]
#
# make calls this on every build, so it has to cost nothing when there is
# nothing to do: every check below is a command -v or a file test, and no
# package manager is reached for unless something is actually missing.
#
# On macOS it uses Homebrew. Elsewhere it says what to install, because
# those need administrator rights, and docs/INSTALL.md has the commands.
#
# It never downloads a model. The app fetches the speech model and the
# language model itself, on first run, which is what a customer does and
# so is what this machine should do too.
set -e
SYSTEM=$(uname -s 2>/dev/null)
FFMPEG_DIR=${1:-.build/ffmpeg}
LLAMA_DIR=${2:-.build/llama}

if [ "$SYSTEM" != Darwin ]; then
	# Nothing to say when nothing is missing, because make calls this every
	# time and a line that is always there is a line nobody reads.
	if ! command -v go >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
		echo "On this system, install the tools as docs/INSTALL.md describes."
		echo "make check shows what is missing."
	fi
	exit 0
fi

# What is missing, gathered first, so Homebrew is started once or not at
# all rather than once per tool.
missing=""
want() { command -v "$1" >/dev/null 2>&1 || missing="$missing $2"; }
want go go
want npm node
# What building our own ffmpeg needs. harfbuzz is built with meson, and
# ffmpeg assembles its own x86 code with nasm.
if [ ! -x "$FFMPEG_DIR/bin/ffmpeg" ]; then
	want meson meson
	want ninja ninja
	want pkg-config pkg-config
	want nasm nasm
fi
# And what building our own llama-server needs. llama.cpp itself is not
# installed from Homebrew any more: a customer has no Homebrew, so the
# llama-server that has to work is the one we build and ship, and the way
# to find out whether it works is to run that one every day.
if [ ! -x "$LLAMA_DIR/bin/llama-server" ]; then
	want cmake cmake
fi

if [ -n "$missing" ]; then
	if ! command -v brew >/dev/null 2>&1; then
		echo "These are missing and Homebrew is not here to install them:$missing"
		echo "Homebrew is at https://brew.sh, or see docs/INSTALL.md."
		exit 1
	fi
	xcode-select -p >/dev/null 2>&1 || {
		echo "Installing the command line tools"
		xcode-select --install
	}
	for formula in $missing; do
		echo "Installing $formula"
		brew install "$formula"
	done
fi

# The ffmpeg framefairy ships, built from source without libx264 so the
# build is LGPL. It takes many minutes and it happens once: after that the
# file is there and this is a file test.
#
# It is built here rather than by hand because what is run every day has to
# be what a customer runs. A build that fails is not a reason to be unable
# to work: the search path answers instead, and the line below says so, so
# that nobody is left wondering which ffmpeg made a clip.
if [ ! -x "$FFMPEG_DIR/bin/ffmpeg" ]; then
	# A build that did not work is not tried again on every make. That would
	# be several minutes of nothing before every build, for as long as it
	# stays broken. It is tried again the moment the script that does it
	# changes, and make ffmpeg always tries again whatever happened.
	recipe=$(cksum scripts/build-ffmpeg.sh | cut -d' ' -f1)
	if [ "$(cat "$FFMPEG_DIR/failed" 2>/dev/null)" = "$recipe" ]; then
		echo "The ffmpeg framefairy ships did not build last time, so the one on the"
		echo "search path is used instead. Try it again with: make ffmpeg"
	else
		echo "Building the ffmpeg framefairy ships. This takes several minutes, once."
		if sh scripts/build-ffmpeg.sh "$FFMPEG_DIR"; then
			rm -f "$FFMPEG_DIR/failed"
		else
			mkdir -p "$FFMPEG_DIR"
			echo "$recipe" >"$FFMPEG_DIR/failed"
			echo
			echo "That did not work, so framefairy will use the ffmpeg on the search path."
			echo "Try it again on its own with: make ffmpeg"
			if ! command -v ffmpeg >/dev/null 2>&1 ||
				! ffmpeg -hide_banner -filters 2>/dev/null | grep -q ' ass '; then
				echo "There is no ffmpeg with libass on the search path either, so installing one."
				brew tap homebrew-ffmpeg/ffmpeg
				brew list --formula ffmpeg >/dev/null 2>&1 && brew unlink ffmpeg
				brew install homebrew-ffmpeg/ffmpeg/ffmpeg
			fi
		fi
	fi
fi

# The llama-server framefairy ships, built from llama.cpp, which is MIT.
# Same rules as ffmpeg above: several minutes, once, and after that this is
# a file test.
#
# It is built here rather than installed from Homebrew because a customer
# has no Homebrew. The llama-server that has to work is the one in the app,
# so that is the one to run every day. A build that fails is not a reason
# to be unable to work: the search path answers instead, and the app says
# plainly when nothing answers at all.
if [ ! -x "$LLAMA_DIR/bin/llama-server" ]; then
	recipe=$(cksum scripts/build-llama.sh | cut -d' ' -f1)
	if [ "$(cat "$LLAMA_DIR/failed" 2>/dev/null)" = "$recipe" ]; then
		echo "The llama-server framefairy ships did not build last time, so a local"
		echo "model needs one on the search path. Try it again with: make llama"
	else
		echo "Building the llama-server framefairy ships. This takes a few minutes, once."
		if sh scripts/build-llama.sh "$LLAMA_DIR"; then
			rm -f "$LLAMA_DIR/failed"
		else
			mkdir -p "$LLAMA_DIR"
			echo "$recipe" >"$LLAMA_DIR/failed"
			echo
			echo "That did not work. A local model will need a llama-server on the search"
			echo "path until it does. Try it again on its own with: make llama"
		fi
	fi
fi
