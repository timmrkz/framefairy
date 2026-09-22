#!/bin/sh
# Builds the llama-server framefairy ships, from source, for the machine it
# runs on.
#
#   scripts/build-llama.sh [OUTDIR]
#
# Why we ship one at all. A local model is half of the local way: the .gguf
# is what answers and llama.cpp's server is what runs it. A customer has no
# Homebrew and no terminal, so without this the app downloads fourteen
# gigabytes and then asks them to go and install a program. That is not a
# product, it is an instruction.
#
# Why we may. llama.cpp is MIT. It is a separate process, the same as
# ffmpeg, so nothing of it is linked into our binary and there is no
# entitlement to ask macOS for. Its licence travels with it, as
# LICENSE-llama.cpp beside the binary. See docs/PACKAGING.md.
#
# This is not run by make on every build. It takes several minutes and its
# answer changes only when this file does, so make runs it once when there
# is no binary, and the workflow that makes a release takes the archive it
# leaves behind.
set -e

# Absolute once, here. Everything below runs from a folder it cd'd into,
# and a relative path would point somewhere different in each. This is the
# bug that made make ffmpeg fail for everyone but the person who wrote it.
OUT=${1:-llama-build}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
WORK="$OUT/work"
JOBS=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
SYSTEM=$(uname -s 2>/dev/null)

# Pinned, because a build nobody can repeat is not a build. Raising this is
# a deliberate act with a real plan generated after it.
LLAMA_VERSION=b11105
LLAMA_REPO=https://github.com/ggml-org/llama.cpp

mkdir -p "$WORK"

say() { printf '\n== %s\n' "$1"; }

# Everything the build says goes to a log rather than to the screen. This is
# tens of thousands of lines of somebody else's C++ and a wall of it buries
# the one line that matters. A step that fails prints the end of the log.
LOG="$OUT/build.log"
: >"$LOG"
finish() {
	code=$?
	if [ "$code" != 0 ]; then
		echo >&2
		echo "build-llama.sh: that step failed. The last of $LOG:" >&2
		tail -40 "$LOG" >&2
	fi
	# Said again on the way out, or the shell reports what tail returned and
	# a build that failed looks like a build that worked.
	exit "$code"
}
trap finish EXIT

for tool in cmake git; do
	command -v "$tool" >/dev/null 2>&1 || {
		echo "build-llama.sh: $tool is needed and is not on this machine." >&2
		echo "On macOS: brew install $tool. Elsewhere see docs/INSTALL.md." >&2
		exit 1
	}
done

SRC="$WORK/llama.cpp-$LLAMA_VERSION"
say "llama.cpp $LLAMA_VERSION"
if [ ! -d "$SRC" ]; then
	echo "  fetching the source of llama.cpp $LLAMA_VERSION"
	# advice.detachedHead off, as in build-ffmpeg.sh: a tag is not a
	# branch and the fourteen lines saying so belong nowhere.
	git -c advice.detachedHead=false clone --depth 1 --branch "$LLAMA_VERSION" \
		-q "$LLAMA_REPO" "$SRC" >>"$LOG" 2>&1
fi

# Every one of these is a decision, and the first two are the ones that
# decide whether this can be shipped at all.
#
# BUILD_SHARED_LIBS is ON by default here, which would leave libllama and
# four ggml libraries beside the program, each needing its own signature and
# its own search path inside the bundle. That is the sherpa-onnx runpath
# problem five more times over. Off, it is one file with nothing to chase,
# the same reason our ffmpeg is static.
#
# GGML_NATIVE is ON by default and compiles for the machine doing the
# building, which for anything we hand to somebody else is a binary that
# dies on their machine with an illegal instruction. Off, it is built for
# the architecture rather than for this particular chip.
#
# The rest is what we do not want: no tests, no examples, no web interface.
# The app talks to the server's HTTP API and never opens its page, and with
# the interface left on the build reaches out to Hugging Face for prebuilt
# assets, which is a release that depends on somebody else's website being
# up.
#
# OpenMP off on purpose: it would link libgomp from the build machine, and
# a borrowed library is exactly what the check at the bottom is for. On
# macOS the work is Metal's anyway.
#
# OpenSSL off for the same reason, and the check at the bottom is how this
# was found: with it on, the finished llama-server named libssl.so.3 and
# libcrypto.so.3 from the build machine, which is a program that runs on
# the build machine and nowhere else. It buys HTTPS, which is there so the
# server can fetch a model from a URL and serve its own page over TLS. We
# hand it a file on disk and talk to it on 127.0.0.1, so there is nothing
# to give up.
FLAGS="-DCMAKE_BUILD_TYPE=Release \
	-DCMAKE_INSTALL_PREFIX=$OUT \
	-DBUILD_SHARED_LIBS=OFF \
	-DGGML_BACKEND_DL=OFF \
	-DGGML_NATIVE=OFF \
	-DGGML_OPENMP=OFF \
	-DLLAMA_BUILD_TESTS=OFF \
	-DLLAMA_BUILD_EXAMPLES=OFF \
	-DLLAMA_BUILD_TOOLS=ON \
	-DLLAMA_BUILD_SERVER=ON \
	-DLLAMA_BUILD_UI=OFF \
	-DLLAMA_USE_PREBUILT_UI=OFF \
	-DLLAMA_OPENSSL=OFF"

if [ "$SYSTEM" = Darwin ]; then
	# Metal is what makes a model answer in seconds on an M2 rather than
	# minutes. EMBED_LIBRARY builds the shaders into the binary: without it
	# there is a .metal file to carry, to find at runtime and to sign, and
	# one file is the whole point.
	#
	# The same macOS floor as the app and as our ffmpeg, so a tool beside
	# the program never wants a newer system than the program does.
	FLAGS="$FLAGS -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON \
		-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0"
	export MACOSX_DEPLOYMENT_TARGET=13.0
fi

say "building"
# shellcheck disable=SC2086
cmake -S "$SRC" -B "$SRC/build" $FLAGS >>"$LOG" 2>&1
cmake --build "$SRC/build" --target llama-server -j "$JOBS" >>"$LOG" 2>&1

mkdir -p "$OUT/bin"
BUILT=$(find "$SRC/build" -name 'llama-server' -type f -perm -u+x | head -1)
if [ -z "$BUILT" ]; then
	echo "build-llama.sh: the build finished and left no llama-server." >&2
	exit 1
fi
cp "$BUILT" "$OUT/bin/llama-server"
# The licence travels with the binary. MIT asks for the notice to go with
# every copy, and a file beside the program is the whole of that.
cp "$SRC/LICENSE" "$OUT/bin/LICENSE-llama.cpp"

say "what came out"
LS="$OUT/bin/llama-server"
"$LS" --version 2>&1 | head -2

# Checked rather than assumed, the same as with ffmpeg, because the flags
# meant to prevent this are exactly what would be wrong.
#
# A build that names a library from this machine runs on this machine and
# nowhere else. The allowed list is the system's own and nothing more:
# llama-server is C++, so libstdc++ belongs here on Linux, and on macOS
# everything must come from /usr/lib or /System/Library, which is where the
# Metal and Accelerate frameworks live.
borrowed=""
case "$SYSTEM" in
Darwin)
	borrowed=$(otool -L "$LS" | tail -n +2 | awk '{print $1}' |
		grep -vE '^(/usr/lib/|/System/Library/)' || true)
	;;
Linux)
	borrowed=$(ldd "$LS" 2>/dev/null | awk '{print $1}' | sed 's:.*/::' |
		grep -vE '^(linux-vdso|ld-linux[^.]*|libc|libm|libdl|libpthread|librt|libz|libgcc_s|libstdc\+\+)\.so' || true)
	;;
esac
if [ -n "$borrowed" ]; then
	echo >&2
	echo "build-llama.sh: this llama-server needs libraries from this machine, so it" >&2
	echo "runs only on this machine:" >&2
	echo "$borrowed" | sed 's/^/  /' >&2
	exit 1
fi

# And on macOS it has to have Metal in it. Without it llama.cpp falls back
# to the cores, which is the difference between a clip search taking a
# minute and one taking twenty, and nothing else here would have said so.
#
# Asked of the binary, by the name ggml's own log lines are compiled with.
# Only when there is something to ask with: a check that cannot be made is
# not a check that failed, and stopping a good build because strings is not
# installed would be worse than not looking.
if [ "$SYSTEM" = Darwin ] && command -v strings >/dev/null 2>&1; then
	if ! strings - "$LS" | grep -q ggml_metal; then
		echo >&2
		echo "build-llama.sh: no Metal in this build, so a model would run on the" >&2
		echo "cores instead of the media engine. Check that GGML_METAL stayed on:" >&2
		echo "  grep -i metal $LOG" >&2
		exit 1
	fi
fi

# What this build is, written down beside the binary. The same idea as the
# one build-ffmpeg.sh leaves: somebody holding only the finished program
# can still say what it is and what it came from.
INFO="$OUT/bin/BUILD-llama.txt"
{
	echo "llama-server, built for framefairy"
	echo
	echo "built    $(date -u '+%Y-%m-%d %H:%M UTC') on $SYSTEM $(uname -m)"
	echo "recipe   scripts/build-llama.sh, cksum $(cksum <"$0" | cut -d' ' -f1)"
	echo "source   $LLAMA_REPO at $LLAMA_VERSION"
	echo "licence  MIT, the notice is in LICENSE-llama.cpp beside this file"
	echo
	echo "version, as the binary itself reports it"
	# It says hello on its way to answering, so only the line that answers.
	"$LS" --version 2>&1 | grep -m1 '^version:' | sed 's/^/  /'
	echo
	echo "cmake"
	# One flag a line, without the tabs this file is written with, and
	# without the install path, which is wherever the build happened to
	# run and would make two identical builds read differently.
	echo "$FLAGS" | tr ' \t' '\n\n' | grep '^-D' |
		grep -v '^-DCMAKE_INSTALL_PREFIX' | sed 's/^/  /'
} >"$INFO"

echo
echo "llama-server  $(du -h "$LS" | cut -f1)  $LS"
echo
echo "MIT, one file, built for the architecture and not for this machine."
echo "What it is and what it came from: $INFO"
