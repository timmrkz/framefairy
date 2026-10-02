#!/bin/sh
# Prints the SHA-256 of each tool make puts beside the programs, as the
# programs take them: name=sum, separated by commas.
#
#   scripts/tool-sums.sh [STAMPS]
#
# make builds this into every program that runs a tool, and a program runs
# a tool beside it only when the file is the one summed here. See FindTool
# in engine/tools.go. A tool that is not there is left out, and a program
# then runs no tool of that name from beside itself.
set -e
STAMPS=${1:-.build}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

out=""
for pair in ffmpeg:"$STAMPS/ffmpeg/bin/ffmpeg" ffprobe:"$STAMPS/ffmpeg/bin/ffprobe" \
	llama-server:"$STAMPS/llama/bin/llama-server"; do
	name=${pair%%:*}
	file=${pair#*:}
	if [ -f "$file" ]; then
		out="$out${out:+,}$name=$(sha256 "$file")"
	fi
done
printf '%s\n' "$out"
