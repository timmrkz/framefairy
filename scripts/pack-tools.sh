#!/bin/sh
# Packs the tools framefairy ships into one archive, with a manifest that
# says what each one is and what it came from.
#
#   scripts/pack-tools.sh [FFMPEG_DIR] [LLAMA_DIR] [OUTDIR]
#
# Why an archive at all, when we build these ourselves. Because where the
# build runs and where it is used are two different machines. Building
# ffmpeg and llama.cpp from source takes half an hour and reaches out to
# five other people's download servers, and their answer changes only when
# our own build scripts change. A release that rebuilt them every time
# would pay that half hour for a result identical to the last one, and
# could fail because somebody else's website was having a bad afternoon.
# So they are built rarely, by hand, and what comes out is kept.
#
# The manifest is the part that matters. ffmpeg is LGPL, which we may ship
# only as long as we can say what it is and hand over what it was built
# from, and llama.cpp is MIT, which asks for its notice to travel along.
# The manifest answers both from the binaries themselves rather than from
# what this script believes: the checksum of each file, the licence as the
# binary reports it, and the configure line ffmpeg keeps inside itself.
#
# scripts/verify-tools.sh reads it back, which is how the ffmpeg inside a
# shipped app is shown to be the ffmpeg this archive holds.
set -e

FFMPEG_DIR=${1:-.build/ffmpeg}
LLAMA_DIR=${2:-.build/llama}
OUT=${3:-.build/dist}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)

SYSTEM=$(uname -s 2>/dev/null | tr 'A-Z' 'a-z')
ARCH=$(uname -m 2>/dev/null)
NAME="framefairy-tools-$SYSTEM-$ARCH"

# macOS has shasum, Linux has sha256sum, and both are in the base system.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

missing=""
for f in "$FFMPEG_DIR/bin/ffmpeg" "$FFMPEG_DIR/bin/ffprobe" "$LLAMA_DIR/bin/llama-server"; do
	[ -x "$f" ] || missing="$missing $f"
done
if [ -n "$missing" ]; then
	echo "pack-tools.sh: nothing to pack, these are not built:$missing" >&2
	echo "Build them with: make ffmpeg  and  make llama" >&2
	exit 1
fi

STAGE="$OUT/$NAME"
rm -rf "$STAGE"
mkdir -p "$STAGE/bin"
cp "$FFMPEG_DIR/bin/ffmpeg" "$FFMPEG_DIR/bin/ffprobe" "$LLAMA_DIR/bin/llama-server" "$STAGE/bin/"
for f in BUILD-ffmpeg.txt LICENSE-ffmpeg.txt; do
	[ -f "$FFMPEG_DIR/bin/$f" ] && cp "$FFMPEG_DIR/bin/$f" "$STAGE/"
done
for f in BUILD-llama.txt LICENSE-llama.cpp; do
	[ -f "$LLAMA_DIR/bin/$f" ] && cp "$LLAMA_DIR/bin/$f" "$STAGE/"
done

# The manifest. Everything in it is read off the files being packed, so it
# cannot describe a build other than the one in the archive.
MANIFEST="$STAGE/manifest.txt"
{
	echo "framefairy, the tools we build and ship"
	echo
	echo "packed   $(date -u '+%Y-%m-%d %H:%M UTC')"
	echo "for      $SYSTEM $ARCH"
	if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
		echo "from     framefairy $(git rev-parse HEAD)"
	fi
	echo
	echo "what is in it, by sha256 of the file itself"
	for f in ffmpeg ffprobe llama-server; do
		printf '  %s  bin/%s\n' "$(sha256 "$STAGE/bin/$f")" "$f"
	done
	echo
	echo "Check a copy of one of these against this list with:"
	echo "  sh scripts/verify-tools.sh <folder holding them> <this file>"
	echo
	for f in BUILD-ffmpeg.txt BUILD-llama.txt; do
		if [ -f "$STAGE/$f" ]; then
			echo "---------------------------------------------------------------"
			cat "$STAGE/$f"
			echo
		fi
	done
} >"$MANIFEST"

(cd "$OUT" && tar -czf "$NAME.tar.gz" "$NAME")
cp "$MANIFEST" "$OUT/$NAME.manifest.txt"
(cd "$OUT" && sha256 "$NAME.tar.gz" >"$NAME.tar.gz.sha256")
rm -rf "$STAGE"

echo "$OUT/$NAME.tar.gz  $(du -h "$OUT/$NAME.tar.gz" | cut -f1)"
echo "$OUT/$NAME.tar.gz.sha256"
echo "$OUT/$NAME.manifest.txt"
