#!/bin/sh
# Builds the ffmpeg framefairy ships, from source, for the machine it runs on.
#
#   scripts/build-ffmpeg.sh [OUTDIR]
#
# Why we build our own rather than take one. ffmpeg is LGPL until it is
# configured with --enable-gpl, and that flag exists to allow GPL-licensed
# external encoders. Checked against ffmpeg's own configure, the list of
# libraries that need it is EXTERNAL_LIBRARY_GPL_LIST, and the only one of
# them anybody wants is libx264. Nearly every prebuilt ffmpeg has libx264,
# so nearly every prebuilt ffmpeg is GPL. Ours does not and is not. The
# encoder comes from the system instead, h264_videotoolbox on macOS. See
# docs/PACKAGING.md.
#
# It is static, so it is one file with nothing to chase. A dynamic build
# would drag freetype, fribidi and harfbuzz along as separate libraries,
# each needing its own signing and its own search path inside the bundle,
# which is the sherpa-onnx runpath problem four more times over.
#
# This is not run by make. It takes many minutes and its answer changes
# only when this file does, so it is run by hand or by the workflow that
# makes a release, and what it leaves behind is kept as an archive.
set -e

# Every library below is built from a folder of its own, with a cd into it,
# so a path that is relative to where this was started points somewhere
# different in each one. make hands this .build/ffmpeg, which is relative,
# and freetype is the one that says so out loud: "expected an absolute
# directory name for --prefix". The others would have installed into the
# wrong place without a word. So it is made absolute once, here, and
# nothing below has to think about it.
OUT=${1:-ffmpeg-build}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
WORK="$OUT/work"
PREFIX="$OUT/deps"
JOBS=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
SYSTEM=$(uname -s 2>/dev/null)

# Pinned, because a build nobody can repeat is not a build. Raising one of
# these is a deliberate act with a test render after it.
FFMPEG_VERSION=7.1.1
FREETYPE_VERSION=2.13.3
FRIBIDI_VERSION=1.0.16
HARFBUZZ_VERSION=10.1.0
LIBASS_VERSION=0.17.3

mkdir -p "$WORK" "$PREFIX"
export PKG_CONFIG_PATH="$PREFIX/lib/pkgconfig:$PREFIX/lib64/pkgconfig"
export CFLAGS="-O2 -fPIC${CFLAGS:+ $CFLAGS}"
export LDFLAGS="${LDFLAGS:-}"
if [ "$SYSTEM" = Darwin ]; then
	# The same floor as the app, so the two agree about which macOS they run
	# on. bin/framefairy-app is built with this and a tool beside it that
	# wanted something newer would be a surprise on an older Mac.
	export MACOSX_DEPLOYMENT_TARGET=13.0
	export CFLAGS="$CFLAGS -mmacosx-version-min=13.0"
	export LDFLAGS="$LDFLAGS -mmacosx-version-min=13.0"
fi

say() { printf '\n== %s\n' "$1"; }

# $1 url, $2 the folder it unpacks to, $3 a git repository and $4 its tag,
# for when the archive cannot be reached.
#
# The second route is not belt and braces. A cloud session reaches GitHub
# and SourceForge and little else, so ffmpeg's own site answers nothing
# there, and a build script that only works on a machine with the open
# internet is a build script nobody can check before a release.
fetch() {
	if [ -d "$WORK/$2" ]; then return 0; fi
	echo "fetching $2"
	if curl -sSL --fail --retry 2 --max-time 300 -o "$WORK/$2.tar" "$1" 2>/dev/null; then
		tar -xf "$WORK/$2.tar" -C "$WORK"
		rm "$WORK/$2.tar"
		return 0
	fi
	rm -f "$WORK/$2.tar"
	if [ -n "$3" ]; then
		echo "  the archive is unreachable, taking $4 from git instead"
		git clone --depth 1 --branch "$4" -q "$3" "$WORK/$2"
		return 0
	fi
	echo "build-ffmpeg.sh: cannot fetch $2 from $1" >&2
	exit 1
}

# freetype and harfbuzz each want the other. The way out is to build
# freetype once without harfbuzz, build harfbuzz against it, then build
# freetype again so text is shaped properly. Skipping the second pass is
# how captions come out with the wrong kerning.
say "freetype, first pass"
fetch "https://downloads.sourceforge.net/freetype/freetype-$FREETYPE_VERSION.tar.xz" "freetype-$FREETYPE_VERSION"
(cd "$WORK/freetype-$FREETYPE_VERSION" &&
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--with-harfbuzz=no --with-brotli=no --with-png=no --with-bzip2=no >/dev/null &&
	make -j"$JOBS" >/dev/null && make install >/dev/null)

say "fribidi"
fetch "https://github.com/fribidi/fribidi/releases/download/v$FRIBIDI_VERSION/fribidi-$FRIBIDI_VERSION.tar.xz" "fribidi-$FRIBIDI_VERSION"
(cd "$WORK/fribidi-$FRIBIDI_VERSION" &&
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--disable-docs >/dev/null &&
	make -j"$JOBS" >/dev/null && make install >/dev/null)

say "harfbuzz"
fetch "https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VERSION/harfbuzz-$HARFBUZZ_VERSION.tar.xz" "harfbuzz-$HARFBUZZ_VERSION"
(cd "$WORK/harfbuzz-$HARFBUZZ_VERSION" &&
	rm -rf build &&
	meson setup build --prefix="$PREFIX" --buildtype=release \
		--default-library=static -Dtests=disabled -Ddocs=disabled \
		-Dcairo=disabled -Dglib=disabled -Dgobject=disabled -Dicu=disabled \
		-Dfreetype=enabled >/dev/null &&
	meson compile -C build >/dev/null 2>&1 &&
	meson install -C build >/dev/null)

say "freetype, second pass, this time with harfbuzz"
(cd "$WORK/freetype-$FREETYPE_VERSION" &&
	make distclean >/dev/null 2>&1 || true
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--with-harfbuzz=yes --with-brotli=no --with-png=no --with-bzip2=no >/dev/null &&
	make -j"$JOBS" >/dev/null && make install >/dev/null)

say "libass"
fetch "https://github.com/libass/libass/releases/download/$LIBASS_VERSION/libass-$LIBASS_VERSION.tar.xz" "libass-$LIBASS_VERSION"
(cd "$WORK/libass-$LIBASS_VERSION" &&
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--disable-fontconfig --disable-require-system-font-provider >/dev/null &&
	make -j"$JOBS" >/dev/null && make install >/dev/null)

# The configure line is the whole point of this script, so it is one place
# and it is readable. No --enable-gpl and no --enable-nonfree, which is
# what makes the result LGPL and redistributable. ffmpeg -L on the finished
# binary is the proof, and this script checks it below rather than trusting
# the flags.
say "ffmpeg $FFMPEG_VERSION"
fetch "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz" "ffmpeg-$FFMPEG_VERSION" \
	"https://github.com/FFmpeg/FFmpeg.git" "n$FFMPEG_VERSION"
EXTRA=""
if [ "$SYSTEM" = Darwin ]; then
	EXTRA="--enable-videotoolbox --enable-audiotoolbox"
fi
(cd "$WORK/ffmpeg-$FFMPEG_VERSION" &&
	./configure \
		--prefix="$OUT" \
		--pkg-config-flags=--static \
		--extra-cflags="-I$PREFIX/include" \
		--extra-ldflags="-L$PREFIX/lib" \
		--enable-static --disable-shared \
		--enable-libass \
		--disable-debug \
		--disable-doc \
		--disable-network \
		--disable-autodetect \
		--enable-zlib \
		--enable-iconv \
		$EXTRA >/dev/null &&
	make -j"$JOBS" >/dev/null &&
	make install >/dev/null)

say "what came out"
FF="$OUT/bin/ffmpeg"
"$FF" -hide_banner -version | head -1
echo
echo "licence:"
"$FF" -hide_banner -L 2>/dev/null | head -3

# Checked rather than assumed. A build that says GPL anywhere is a build
# that cannot ship, and finding that out here costs nothing.
if "$FF" -hide_banner -L 2>/dev/null | head -5 | grep -qi "GNU General Public"; then
	echo >&2
	echo "build-ffmpeg.sh: this came out GPL, which cannot ship. Check the configure line." >&2
	exit 1
fi
if "$FF" -hide_banner -encoders 2>/dev/null | awk '{print $2}' | grep -qx libx264; then
	echo >&2
	echo "build-ffmpeg.sh: libx264 got in, which makes the build GPL." >&2
	exit 1
fi
# libass is what burns the captions in. Without it the app renders shorts
# with no words on them, which is worse than not rendering at all.
if ! "$FF" -hide_banner -filters 2>/dev/null | awk '{print $2}' | grep -qx subtitles; then
	echo >&2
	echo "build-ffmpeg.sh: no subtitles filter, so libass did not get in." >&2
	exit 1
fi

echo
echo "ffmpeg  $(du -h "$FF" | cut -f1)  $FF"
echo "ffprobe $(du -h "$OUT/bin/ffprobe" | cut -f1)  $OUT/bin/ffprobe"
echo
echo "LGPL, no libx264, subtitles filter present."
