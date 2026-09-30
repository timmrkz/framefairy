#!/bin/sh
# Packs the source the shipped tools were built from.
#
#   scripts/pack-source.sh [OUTDIR]
#
# ffmpeg is LGPL. We may ship it because it is built without libx264, and
# the obligation that comes with shipping it is that anyone who has the
# binary can get the source it was built from. This is that source: the
# five upstream archives at the versions the build pins, plus the scripts
# that built them, in one file that goes on the same release page as the
# binary.
#
# It is fetched fresh rather than taken out of a build folder, so what is
# in here is the untouched upstream archive and not a tree somebody has
# already compiled in. The versions and their pins, a sha256 for each
# archive and a commit for each clone, are read out of build-ffmpeg.sh and
# build-llama.sh and checked here too, so what is packed is provably what
# was built.
#
# The same archive carries llama.cpp, which is MIT and asks for less, but
# there is no sense in having two answers to the same question.
set -e

OUT=${1:-.build/dist}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
WORK="$OUT/source"
rm -rf "$WORK"
mkdir -p "$WORK"

# Read, never repeated. A version written twice is a version that is wrong
# in one of the two places.
version() { grep -m1 "^$1=" "$2" | cut -d= -f2; }
FFMPEG_VERSION=$(version FFMPEG_VERSION scripts/build-ffmpeg.sh)
FREETYPE_VERSION=$(version FREETYPE_VERSION scripts/build-ffmpeg.sh)
FRIBIDI_VERSION=$(version FRIBIDI_VERSION scripts/build-ffmpeg.sh)
HARFBUZZ_VERSION=$(version HARFBUZZ_VERSION scripts/build-ffmpeg.sh)
LIBASS_VERSION=$(version LIBASS_VERSION scripts/build-ffmpeg.sh)
LLAMA_VERSION=$(version LLAMA_VERSION scripts/build-llama.sh)
FFMPEG_COMMIT=$(version FFMPEG_COMMIT scripts/build-ffmpeg.sh)
FREETYPE_SHA256=$(version FREETYPE_SHA256 scripts/build-ffmpeg.sh)
FRIBIDI_SHA256=$(version FRIBIDI_SHA256 scripts/build-ffmpeg.sh)
HARFBUZZ_SHA256=$(version HARFBUZZ_SHA256 scripts/build-ffmpeg.sh)
LIBASS_SHA256=$(version LIBASS_SHA256 scripts/build-ffmpeg.sh)
LLAMA_COMMIT=$(version LLAMA_COMMIT scripts/build-llama.sh)
for v in "$FFMPEG_VERSION" "$FREETYPE_VERSION" "$FRIBIDI_VERSION" \
	"$HARFBUZZ_VERSION" "$LIBASS_VERSION" "$LLAMA_VERSION" "$FFMPEG_COMMIT" \
	"$FREETYPE_SHA256" "$FRIBIDI_SHA256" "$HARFBUZZ_SHA256" "$LIBASS_SHA256" \
	"$LLAMA_COMMIT"; do
	[ -n "$v" ] || {
		echo "pack-source.sh: a version could not be read out of the build scripts." >&2
		exit 1
	}
done

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# $1 the file to write, $2 the url. Quiet on success, because six lines
# saying a download worked is six lines nobody reads.
try() {
	curl -sSL --fail --retry 2 --max-time 600 -o "$WORK/$1" "$2" || {
		rm -f "$WORK/$1"
		return 1
	}
	echo "  $1"
}
# $3 the archive's sha256, as build-ffmpeg.sh pins it.
get() {
	try "$1" "$2" || {
		echo "pack-source.sh: cannot fetch $1 from $2" >&2
		exit 1
	}
	got=$(sha256 "$WORK/$1")
	[ "$got" = "$3" ] || {
		echo "pack-source.sh: $2 is not the archive build-ffmpeg.sh pins." >&2
		echo "  expected sha256 $3, got $got" >&2
		exit 1
	}
}

# $1 a name, $2 a git repository, $3 a tag, $4 the commit it has to be. For the two projects that are
# reached that way rather than as a published archive. The .git folder is
# left out: what this archive is for is the source, and the history of a
# project nobody here is going to change is a few hundred megabytes of
# nothing to do with it.
from_git() {
	rm -rf "$WORK/$1"
	# advice.detachedHead off: a tag is not a branch and nobody here is
	# going to commit to it, so fourteen lines telling us so are fourteen
	# lines of noise.
	git -c advice.detachedHead=false clone --depth 1 --branch "$3" -q "$2" "$WORK/$1" || {
		echo "pack-source.sh: cannot clone $2 at $3" >&2
		exit 1
	}
	got=$(git -C "$WORK/$1" rev-parse HEAD)
	[ "$got" = "$4" ] || {
		echo "pack-source.sh: $3 of $2 is not the commit the build pins." >&2
		echo "  expected $4, got $got" >&2
		exit 1
	}
	rm -rf "$WORK/$1/.git"
	(cd "$WORK" && tar -czf "$1.tar.gz" "$1" && rm -rf "$1")
	echo "  $1.tar.gz"
}

echo "Fetching the source the shipped tools are built from"
# ffmpeg is built from its tag out of git, at the commit build-ffmpeg.sh
# pins, so its source is packed from the same commit.
from_git "FFmpeg-n$FFMPEG_VERSION" \
	"https://github.com/FFmpeg/FFmpeg.git" "n$FFMPEG_VERSION" "$FFMPEG_COMMIT"
get "freetype-$FREETYPE_VERSION.tar.xz" \
	"https://downloads.sourceforge.net/freetype/freetype-$FREETYPE_VERSION.tar.xz" "$FREETYPE_SHA256"
get "fribidi-$FRIBIDI_VERSION.tar.xz" \
	"https://github.com/fribidi/fribidi/releases/download/v$FRIBIDI_VERSION/fribidi-$FRIBIDI_VERSION.tar.xz" "$FRIBIDI_SHA256"
get "harfbuzz-$HARFBUZZ_VERSION.tar.xz" \
	"https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VERSION/harfbuzz-$HARFBUZZ_VERSION.tar.xz" "$HARFBUZZ_SHA256"
get "libass-$LIBASS_VERSION.tar.xz" \
	"https://github.com/libass/libass/releases/download/$LIBASS_VERSION/libass-$LIBASS_VERSION.tar.xz" "$LIBASS_SHA256"
# llama.cpp publishes no source archive of its own, so it is the tag, the
# same one build-llama.sh clones.
from_git "llama.cpp-$LLAMA_VERSION" \
	"https://github.com/ggml-org/llama.cpp.git" "$LLAMA_VERSION" "$LLAMA_COMMIT"

# The recipes go in too. Upstream source on its own does not say how it was
# configured, and the configure line is the whole of why this ffmpeg may be
# shipped at all.
mkdir -p "$WORK/scripts"
cp scripts/build-ffmpeg.sh scripts/build-llama.sh "$WORK/scripts/"

{
	echo "The source framefairy's shipped tools are built from"
	echo
	echo "packed $(date -u '+%Y-%m-%d %H:%M UTC')"
	echo
	echo "Build these with the scripts in scripts/, which are the ones used:"
	echo "  sh scripts/build-ffmpeg.sh <a folder>"
	echo "  sh scripts/build-llama.sh <a folder>"
	echo
	echo "ffmpeg is LGPL 2.1. It is configured without --enable-gpl and"
	echo "without libx264, which is what makes that true, and the finished"
	echo "binary reports its own licence and its own configure line:"
	echo "  ffmpeg -L"
	echo "  ffmpeg -buildconf"
	echo
	echo "llama.cpp is MIT."
	echo
	echo "what is in here, by sha256"
	for f in "$WORK"/*.tar.*; do
		printf '  %s  %s\n' "$(sha256 "$f")" "$(basename "$f")"
	done
} >"$WORK/README.txt"

NAME="framefairy-tools-source"
(cd "$OUT" && tar -czf "$NAME.tar.gz" source)
(cd "$OUT" && sha256 "$NAME.tar.gz" >"$NAME.tar.gz.sha256")
rm -rf "$WORK"

echo
echo "$OUT/$NAME.tar.gz  $(du -h "$OUT/$NAME.tar.gz" | cut -f1)"
echo "$OUT/$NAME.tar.gz.sha256"
