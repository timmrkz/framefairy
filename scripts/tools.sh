#!/bin/sh
# Everything this machine needs to build and run framefairy, installed
# where it is missing.
#
#   scripts/tools.sh [FFMPEG_DIR]
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
want llama-server llama.cpp
# What building our own ffmpeg needs. harfbuzz is built with meson, and
# ffmpeg assembles its own x86 code with nasm.
if [ ! -x "$FFMPEG_DIR/bin/ffmpeg" ]; then
	want meson meson
	want ninja ninja
	want pkg-config pkg-config
	want nasm nasm
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
	echo "Building the ffmpeg framefairy ships. This takes several minutes, once."
	if ! sh scripts/build-ffmpeg.sh "$FFMPEG_DIR"; then
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
