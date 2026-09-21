#!/bin/sh
# Installs the system tools framefairy needs, where they are missing. Only runs
# when you call make tools. On macOS it uses Homebrew. Elsewhere it prints
# the commands, because they need administrator rights.
set -e
SYSTEM=$(uname -s 2>/dev/null)

if [ "$SYSTEM" != Darwin ]; then
	echo "On this system, install the tools as docs/INSTALL.md describes."
	echo "make check shows what is missing."
	exit 0
fi

if ! command -v brew >/dev/null 2>&1; then
	echo "Homebrew is needed first: https://brew.sh"
	exit 1
fi
xcode-select -p >/dev/null 2>&1 || { echo "Installing the command line tools"; xcode-select --install; }

need() { command -v "$1" >/dev/null 2>&1 || { echo "Installing $2"; brew install "$2"; }; }
need go go
need llama-server llama.cpp
need npm node

# The default ffmpeg formula has no libass. The ffmpeg tap has it.
if ! command -v ffmpeg >/dev/null 2>&1 ||
	! ffmpeg -hide_banner -filters 2>/dev/null | grep -q ' ass '; then
	echo "Installing ffmpeg with libass from the ffmpeg tap"
	brew tap homebrew-ffmpeg/ffmpeg
	if brew list --formula ffmpeg >/dev/null 2>&1; then
		brew unlink ffmpeg
	fi
	brew install homebrew-ffmpeg/ffmpeg/ffmpeg
fi

echo "Tools are in place. make models downloads the models."
