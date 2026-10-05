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

# The ffmpeg and the llama-server framefairy ships, built from source: ffmpeg
# without libx264 so the build is LGPL, llama-server from llama.cpp, which
# is MIT. Each takes minutes and happens once for each version: after that
# the file is there and this is a file test and a checksum.
#
# They are built here rather than by hand because what is run every day has
# to be what a customer runs. A build that fails stops make. It used to
# leave the search path to answer, with Homebrew's ffmpeg, or a third
# party's installed for the purpose, which made a broken build look like a
# working one with somebody else's tools in it. The programs take no tool
# from the search path any more, so there is nothing for it to answer.
#
# A tool is built again when the script that builds it is not the one it
# was built with, which is what raising a pin is. It used to be built only
# when it was missing, so a Mac that pulled a new pin went on with the old
# tool and said nothing. Which script built it is in built beside it, a
# checksum of the file, which costs a read of one small file.
#
# A build that did not work is not tried again on every make. That would be
# minutes of nothing before every build, for as long as it stays broken. It
# is tried again the moment the script that does it changes, and make
# ffmpeg and make llama always try again whatever happened.
build_tool() {
	name=$1 dir=$2 script=$3 target=$4
	recipe=$(cksum "$script" | cut -d' ' -f1)
	if [ -x "$dir/bin/$name" ] && [ "$(cat "$dir/built" 2>/dev/null)" = "$recipe" ]; then
		return 0
	fi
	if [ -x "$dir/bin/$name" ]; then
		echo "$script has changed since the $name framefairy ships was built."
		rm -rf "$dir"
	fi
	if [ "$(cat "$dir/failed" 2>/dev/null)" = "$recipe" ]; then
		echo "The $name framefairy ships did not build last time, and no other is used."
		echo "Try it again with: make $target"
		exit 1
	fi
	echo "Building the $name framefairy ships. This takes several minutes, once."
	if sh "$script" "$dir"; then
		rm -f "$dir/failed"
		echo "$recipe" >"$dir/built"
		return 0
	fi
	mkdir -p "$dir"
	echo "$recipe" >"$dir/failed"
	echo
	echo "The $name build did not work, and no other is used. Try it again with: make $target"
	exit 1
}
build_tool ffmpeg "$FFMPEG_DIR" scripts/build-ffmpeg.sh ffmpeg
build_tool llama-server "$LLAMA_DIR" scripts/build-llama.sh llama
