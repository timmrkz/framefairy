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
# Only our own libraries, and nothing this machine happens to have.
#
# PKG_CONFIG_LIBDIR and not PKG_CONFIG_PATH: PATH is looked at first and the
# system is still looked at after it, LIBDIR replaces the search entirely.
# That is the difference between preferring ours and using only ours, and it
# is not a fine distinction. libass takes libunibreak when it can see one,
# and on a Mac with Homebrew it could, so the ffmpeg that came out named
# /opt/homebrew/opt/libunibreak/lib/libunibreak.7.dylib and would not have
# run on any machine without Homebrew. The whole point of building this one
# is that it is one file with nothing to chase.
# --libdir=lib is given to meson below for the same reason this is a short
# list: on Debian and Ubuntu meson installs into lib/x86_64-linux-gnu by
# itself, and then the next library in the chain cannot find it. Which is
# how the leak stayed hidden: freetype could not see our harfbuzz, so it
# quietly took the system's.
export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig:$PREFIX/lib64/pkgconfig"
export PKG_CONFIG_PATH="$PKG_CONFIG_LIBDIR"
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

# Everything the five builds say goes to a log rather than to the screen.
# Building somebody else's C is thousands of warnings about code that is not
# ours to fix, and a wall of them buries the one line that matters. When a
# step fails, the end of the log is what is printed.
LOG="$OUT/build.log"
: >"$LOG"
finish() {
	code=$?
	if [ "$code" != 0 ]; then
		echo >&2
		echo "build-ffmpeg.sh: that step failed. The last of $LOG:" >&2
		tail -40 "$LOG" >&2
	fi
	# Said again on the way out. A trap that falls off the end leaves the
	# shell reporting whatever the last command in the trap returned, which
	# is tail, which works, so a build that failed reported success and
	# make believed it.
	exit "$code"
}
trap finish EXIT

# $1 url, $2 the folder it unpacks to, $3 a git repository and $4 its tag,
# for when the archive cannot be reached.
#
# The second route is not belt and braces. A cloud session reaches GitHub
# and SourceForge and little else, so ffmpeg's own site answers nothing
# there, and a build script that only works on a machine with the open
# internet is a build script nobody can check before a release.
fetch() {
	if [ -d "$WORK/$2" ]; then return 0; fi
	echo "  fetching the source of $2"
	if curl -sSL --fail --retry 2 --max-time 300 -o "$WORK/$2.tar" "$1" 2>/dev/null; then
		tar -xf "$WORK/$2.tar" -C "$WORK"
		rm "$WORK/$2.tar"
		return 0
	fi
	rm -f "$WORK/$2.tar"
	if [ -n "$3" ]; then
		echo "  the archive is unreachable, taking $4 from git instead"
		# advice.detachedHead off: fourteen lines about a state nobody
		# here is going to commit in, on the screen, in the middle of a
		# build.
		git -c advice.detachedHead=false clone --depth 1 --branch "$4" -q \
			"$3" "$WORK/$2" >>"$LOG" 2>&1
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
		--with-harfbuzz=no --with-brotli=no --with-png=no --with-bzip2=no &&
	make -j"$JOBS" && make install) >>"$LOG" 2>&1

say "fribidi"
fetch "https://github.com/fribidi/fribidi/releases/download/v$FRIBIDI_VERSION/fribidi-$FRIBIDI_VERSION.tar.xz" "fribidi-$FRIBIDI_VERSION"
(cd "$WORK/fribidi-$FRIBIDI_VERSION" &&
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--disable-docs &&
	make -j"$JOBS" && make install) >>"$LOG" 2>&1

say "harfbuzz"
fetch "https://github.com/harfbuzz/harfbuzz/releases/download/$HARFBUZZ_VERSION/harfbuzz-$HARFBUZZ_VERSION.tar.xz" "harfbuzz-$HARFBUZZ_VERSION"
(cd "$WORK/harfbuzz-$HARFBUZZ_VERSION" &&
	rm -rf build &&
	meson setup build --prefix="$PREFIX" --libdir=lib --buildtype=release \
		--default-library=static -Dtests=disabled -Ddocs=disabled \
		-Dcairo=disabled -Dglib=disabled -Dgobject=disabled -Dicu=disabled \
		-Dfreetype=enabled &&
	meson compile -C build &&
	meson install -C build) >>"$LOG" 2>&1

say "freetype, second pass, this time with harfbuzz"
(cd "$WORK/freetype-$FREETYPE_VERSION" &&
	make distclean >/dev/null 2>&1 || true
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--with-harfbuzz=yes --with-brotli=no --with-png=no --with-bzip2=no &&
	make -j"$JOBS" && make install) >>"$LOG" 2>&1

say "libass"
fetch "https://github.com/libass/libass/releases/download/$LIBASS_VERSION/libass-$LIBASS_VERSION.tar.xz" "libass-$LIBASS_VERSION"
(cd "$WORK/libass-$LIBASS_VERSION" &&
	./configure --prefix="$PREFIX" --enable-static --disable-shared \
		--disable-fontconfig --disable-require-system-font-provider &&
	make -j"$JOBS" && make install) >>"$LOG" 2>&1

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
		$EXTRA &&
	make -j"$JOBS" &&
	make install) >>"$LOG" 2>&1

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

# And it has to be a file that runs on somebody else's machine. A static
# build may still name a library from this one: libass takes libunibreak
# when it can see one, and on a Mac with Homebrew it could, so the binary
# came out naming a dylib under /opt/homebrew and would have failed on the
# first render for anyone without it. The libraries it may name are the
# system's own and nothing else.
#
# It is read back rather than assumed, the same as the licence above,
# because the flags that were meant to prevent it are exactly what was
# wrong the first time.
borrowed=""
case "$SYSTEM" in
Darwin)
	borrowed=$(otool -L "$FF" | tail -n +2 | awk '{print $1}' |
		grep -vE '^(/usr/lib/|/System/Library/)' || true)
	;;
Linux)
	# Everything but the C runtime and what a C++ library needs. libass,
	# freetype, harfbuzz and fribidi are ours and static, so seeing any of
	# them named here means the system's was used in their place. That is
	# what was happening: the Linux build named libharfbuzz, libfreetype
	# and libglib from /lib, so it was neither static nor ours.
	# zlib is asked for on purpose and is on every Linux and every Mac, so
	# it belongs here with the C runtime. And the loader is named
	# ld-linux-x86-64.so.2 or ld-linux-aarch64.so.1, never plain ld-linux,
	# which the first version of this list did not allow for and then
	# reported the loader itself as something borrowed.
	borrowed=$(ldd "$FF" 2>/dev/null | awk '{print $1}' | sed 's:.*/::' |
		grep -vE '^(linux-vdso|ld-linux[^.]*|libc|libm|libdl|libpthread|librt|libz|libgcc_s|libstdc\+\+)\.so' || true)
	;;
esac
if [ -n "$borrowed" ]; then
	echo >&2
	echo "build-ffmpeg.sh: this ffmpeg needs libraries from this machine, so it runs" >&2
	echo "only on this machine:" >&2
	echo "$borrowed" | sed 's/^/  /' >&2
	exit 1
fi

# The licence text travels with the binary. LGPL asks for that and it is
# one file, so there is nothing to weigh up.
cp "$WORK/ffmpeg-$FFMPEG_VERSION/COPYING.LGPLv2.1" "$OUT/bin/LICENSE-ffmpeg.txt"

# What this build is, written down beside the binary, read back off the
# binary rather than echoed from the variables above.
#
# The point of this file is that somebody who has only the finished ffmpeg
# can still answer the two questions that matter: what licence is it under,
# and what was it built from. -buildconf is ffmpeg's own record of its
# configure line, kept inside the executable, so it cannot drift from what
# this script did the way a copy of the line would.
INFO="$OUT/bin/BUILD-ffmpeg.txt"
{
	echo "ffmpeg, built for framefairy"
	echo
	echo "built    $(date -u '+%Y-%m-%d %H:%M UTC') on $SYSTEM $(uname -m)"
	echo "recipe   scripts/build-ffmpeg.sh, cksum $(cksum <"$0" | cut -d' ' -f1)"
	echo "sources  ffmpeg $FFMPEG_VERSION, freetype $FREETYPE_VERSION,"
	echo "         fribidi $FRIBIDI_VERSION, harfbuzz $HARFBUZZ_VERSION, libass $LIBASS_VERSION"
	echo
	echo "version"
	"$FF" -hide_banner -version 2>/dev/null | head -1 | sed 's/^/  /'
	echo
	echo "licence, as the binary itself reports it"
	"$FF" -hide_banner -L 2>/dev/null | head -4 | sed 's/^/  /'
	echo
	echo "configure, as the binary itself reports it"
	"$FF" -hide_banner -buildconf 2>/dev/null | sed 's/^/  /'
} >"$INFO"

echo
echo "ffmpeg  $(du -h "$FF" | cut -f1)  $FF"
echo "ffprobe $(du -h "$OUT/bin/ffprobe" | cut -f1)  $OUT/bin/ffprobe"
echo
echo "LGPL, no libx264, subtitles filter present."
echo "What it is and what it came from: $INFO"
