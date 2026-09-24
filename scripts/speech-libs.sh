#!/bin/sh
# Prints the folder holding the speech libraries to ship, fetching them the
# first time.
#
#   speech-libs.sh
#
# The sherpa-onnx Go module brings prebuilt libraries, and they have
# espeak-ng compiled into them, for speech synthesis. espeak-ng is GPL 3,
# and a GPL 3 library shipped inside a closed, paid app is what that
# licence does not allow, whether the app ever calls it or not. Frame
# Fairy never synthesises speech.
#
# sherpa-onnx publishes the same release built without speech synthesis,
# the -no-tts archives. There every synthesis function is still there and
# only says "TTS is not enabled", so the Go module works against them
# unchanged: the program is built against the module's libraries as ever,
# and these are the ones that go beside it. Both were compared function by
# function, and the Go module calls nothing these do not have.
#
# The archive is pinned by its sha256, and what is inside is read back:
# a library that still carries espeak-ng is refused. See
# docs/THIRD_PARTY.md.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION=$(awk '/k2-fsa\/sherpa-onnx-go /{ print $2; exit }' "$ROOT/go.mod")

case "$(uname -s)-$(uname -m)" in
Darwin-arm64)
	NAME=sherpa-onnx-$VERSION-osx-arm64-shared-no-tts-lib
	SUM=f3e0cbd86cc3f38dad30c97921b40e9a8bcc6f2c943777eb76ad77176993e417
	LIB=libsherpa-onnx-c-api.dylib
	;;
Linux-x86_64)
	NAME=sherpa-onnx-$VERSION-linux-x64-shared-no-tts-lib
	SUM=bf2d998c8b07012cd5098f3b92673bc1333fd9b927767d7cb664be8190d8bc0b
	LIB=libsherpa-onnx-c-api.so
	;;
*)
	# Nothing ships from here yet. The module's own libraries do for
	# running on this machine, and they are not fit to give to anybody.
	echo "speech-libs.sh: no speech library without espeak-ng is pinned for" >&2
	echo "  $(uname -s) $(uname -m), so this build carries the module's, which is" >&2
	echo "  not fit to ship. See docs/THIRD_PARTY.md." >&2
	exit 3
	;;
esac

DIR="$ROOT/.build/speech"
OUT="$DIR/$NAME/lib"
STAMP="$DIR/$NAME.ok"

# Checked once, when it arrives. After that a file test is the whole cost,
# because make asks this on every build.
if [ -f "$STAMP" ] && [ -f "$OUT/$LIB" ]; then
	echo "$OUT"
	exit 0
fi

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

mkdir -p "$DIR"
ARCHIVE="$DIR/$NAME.tar.bz2"
if [ ! -f "$ARCHIVE" ] || [ "$(sha256 "$ARCHIVE")" != "$SUM" ]; then
	echo "Fetching the speech library without speech synthesis, $NAME" >&2
	curl -sSL --fail --retry 2 --max-time 600 -o "$ARCHIVE.part" \
		"https://github.com/k2-fsa/sherpa-onnx/releases/download/$VERSION/$NAME.tar.bz2"
	mv "$ARCHIVE.part" "$ARCHIVE"
fi
got=$(sha256 "$ARCHIVE")
if [ "$got" != "$SUM" ]; then
	echo "speech-libs.sh: $NAME.tar.bz2 is not the file that was checked." >&2
	echo "  expected $SUM" >&2
	echo "  got      $got" >&2
	rm -f "$ARCHIVE"
	exit 1
fi

rm -rf "${DIR:?}/$NAME"
tar xjf "$ARCHIVE" -C "$DIR"
if [ ! -f "$OUT/$LIB" ]; then
	echo "speech-libs.sh: $NAME.tar.bz2 holds no $LIB." >&2
	exit 1
fi
# Read back rather than trusted by its name.
if grep -aq 'espeak-ng' "$OUT/$LIB"; then
	echo "speech-libs.sh: $OUT/$LIB still carries espeak-ng." >&2
	exit 1
fi
touch "$STAMP"
echo "$OUT"
