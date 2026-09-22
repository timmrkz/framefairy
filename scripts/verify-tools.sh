#!/bin/sh
# Says whether the tools in a folder are the ones an archive's manifest
# describes.
#
#   scripts/verify-tools.sh [FOLDER] [MANIFEST]
#
# The folder holds ffmpeg, ffprobe and llama-server: bin/ during
# development, framefairy.app/Contents/MacOS in a shipped app. The manifest
# is the one scripts/pack-tools.sh wrote beside the archive.
#
# This exists because "we build our own ffmpeg" is a claim, and a claim is
# not a licence position. ffmpeg is ours to ship because it is LGPL and
# built without libx264, and the only way that means anything is if the
# file inside the app can be shown to be the file that was built that way.
# A checksum is the whole of it: the manifest holds one per binary, taken
# from the file itself, and this compares them.
set -e

DIR=${1:-bin}
MANIFEST=${2:-}
if [ -z "$MANIFEST" ]; then
	# The newest one lying about, so the usual case takes no arguments.
	MANIFEST=$(ls -t .build/dist/*.manifest.txt 2>/dev/null | head -1 || true)
fi
if [ -z "$MANIFEST" ] || [ ! -f "$MANIFEST" ]; then
	echo "verify-tools.sh: no manifest. Pass one, or run scripts/pack-tools.sh first." >&2
	exit 1
fi

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

echo "against $MANIFEST"
echo
bad=0
found=0
for tool in ffmpeg ffprobe llama-server; do
	file="$DIR/$tool"
	want=$(awk -v t="bin/$tool" '$2 == t {print $1}' "$MANIFEST" | head -1)
	if [ -z "$want" ]; then
		printf '  ?  %-14s the manifest does not mention it\n' "$tool"
		continue
	fi
	if [ ! -f "$file" ]; then
		printf '  -  %-14s not in %s\n' "$tool" "$DIR"
		continue
	fi
	found=$((found + 1))
	got=$(sha256 "$file")
	if [ "$got" = "$want" ]; then
		printf '  ok %-14s %s\n' "$tool" "$got"
	else
		printf '  NO %-14s\n' "$tool"
		printf '     the manifest says %s\n' "$want"
		printf '     this file is      %s\n' "$got"
		bad=$((bad + 1))
	fi
done

echo
if [ "$found" = 0 ]; then
	echo "Nothing to check. $DIR holds none of these." >&2
	exit 1
fi
if [ "$bad" != 0 ]; then
	echo "$bad of them is not what the manifest describes." >&2
	echo "So whatever the manifest says about the licence does not cover it." >&2
	exit 1
fi
echo "All $found are the ones the manifest describes."
