#!/bin/sh
# Checks what this machine needs for framefairy and says how to get what is
# missing. It changes nothing.
#
#   check.sh              everything
#   check.sh --toolchain  only what building needs, stops on a problem
#   check.sh --quiet      everything, but prints only what is missing
#   check.sh --copy-dlls DIR   Windows: puts the speech library next to the programs

MIN_GO_MINOR=27
MODELS="${HOME}/.framefairy/models"
# Where make puts the programs, and the ffmpeg it puts beside them.
BESIDE="${FRAMEFAIRY_BIN:-bin}"
ASR_DIR="${FRAMEFAIRY_ASR_MODEL:-$MODELS/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8}"
SYSTEM=$(uname -s 2>/dev/null)
missing=0

ok() { [ "$QUIET" = 1 ] || printf '  ok       %s\n' "$1"; }
bad() { missing=1; printf '  missing  %s\n           %s\n' "$1" "$2"; }

if [ "$1" = "--copy-dlls" ]; then
	lib="$(go env GOMODCACHE)/github.com/k2-fsa/sherpa-onnx-go-windows@v1.13.8/lib/x86_64-pc-windows-gnu"
	cp "$lib"/*.dll "$2"/
	exit 0
fi

install_hint() {
	case "$SYSTEM" in
	Darwin) echo "$1" ;;
	Linux) echo "$2" ;;
	*) echo "$3" ;;
	esac
}

toolchain() {
	if ! command -v go >/dev/null 2>&1; then
		bad "Go 1.$MIN_GO_MINOR or newer" "$(install_hint 'brew install go' 'Go from https://go.dev/dl' 'winget install GoLang.Go')"
		return
	fi
	version=$(go env GOVERSION | sed 's/^go//')
	minor=$(echo "$version" | cut -d. -f2)
	if [ "$(echo "$version" | cut -d. -f1)" = 1 ] && [ "${minor:-0}" -lt "$MIN_GO_MINOR" ]; then
		bad "Go 1.$MIN_GO_MINOR or newer, found $version" "$(install_hint 'brew upgrade go' 'Go from https://go.dev/dl' 'winget upgrade GoLang.Go')"
	else
		ok "Go $version"
	fi
	cc=$(go env CC)
	if command -v "${cc:-cc}" >/dev/null 2>&1; then
		ok "C compiler ($cc)"
	else
		bad "a C compiler" "$(install_hint 'xcode-select --install' 'sudo apt install build-essential' 'MSYS2 with gcc, see docs/INSTALL.md')"
	fi
	if command -v npm >/dev/null 2>&1; then
		ok "Node.js"
	else
		bad "Node.js, for the app's interface" "$(install_hint 'brew install node' 'sudo apt install nodejs npm' 'winget install OpenJS.NodeJS.LTS')"
	fi
	if [ "$SYSTEM" = Linux ] && ! pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
		bad "GTK 4 and WebKitGTK 6, for the app" "sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev gstreamer1.0-libav gstreamer1.0-plugins-good"
	fi
	# The speech library travels with the program rather than being found
	# in the Go module cache. On Linux that needs patchelf, on macOS
	# install_name_tool comes with the command line tools.
	if [ "$SYSTEM" = Linux ]; then
		if command -v patchelf >/dev/null 2>&1; then
			ok "patchelf"
		else
			bad "patchelf, to carry the speech library with the program" "sudo apt install patchelf"
		fi
	fi
}

runtime() {
	# The programs look beside themselves before they look at the search
	# path, so this looks in the same order. ffmpeg is named by where it
	# came from, because which one made a clip is a thing worth knowing.
	whose="on the search path"
	ff=$(command -v ffmpeg 2>/dev/null || true)
	if [ -x "$BESIDE/ffmpeg" ]; then
		ff="$BESIDE/ffmpeg"
		whose="ours, from make"
	fi
	if [ -n "$ff" ]; then
		# libass draws the captions. H.264 is not asked of the build any
		# more: our own has no encoder of its own and reaches for the
		# system's, which is h264_videotoolbox on macOS.
		if "$ff" -hide_banner -filters 2>/dev/null | grep -q ' ass '; then
			ok "ffmpeg with libass ($whose)"
		else
			bad "an ffmpeg with libass, the one $whose has none" "$(install_hint 'make' 'see docs/INSTALL.md, step 2' 'see docs/INSTALL.md, step 2')"
		fi
		if ! "$ff" -hide_banner -encoders 2>/dev/null | grep -qE 'h264_videotoolbox|libx264|h264_vaapi|libopenh264'; then
			bad "an H.264 encoder in that ffmpeg" "on macOS h264_videotoolbox comes with the system. On Linux an LGPL build has none yet, see docs/PACKAGING.md"
		fi
	else
		bad "ffmpeg" "$(install_hint 'make' 'sudo apt install ffmpeg' 'winget install Gyan.FFmpeg')"
	fi
	# The same order, and named the same way. A local model is only half
	# of the local way: this is what runs it.
	whose="on the search path"
	ls=$(command -v llama-server 2>/dev/null || true)
	if [ -x "$BESIDE/llama-server" ]; then
		ls="$BESIDE/llama-server"
		whose="ours, from make"
	fi
	if [ -n "$ls" ]; then
		ok "llama-server ($whose)"
	else
		bad "llama-server, which runs a local model" "$(install_hint 'make' 'make llama, see docs/INSTALL.md' 'winget install llama.cpp')"
	fi
	# The models are the app's to fetch, on its first run, the way a
	# customer gets them. make models is still there for the command line,
	# which has no window to ask in.
	if [ -f "$ASR_DIR/tokens.txt" ]; then
		ok "speech model"
	else
		bad "the speech model in $ASR_DIR" "the app fetches it on its first run. For the command line: make models"
	fi
	if ls "$MODELS"/*.gguf >/dev/null 2>&1; then
		ok "language model"
	else
		bad "a language model in $MODELS" "the app fetches one on its first run, or use an Anthropic API key instead"
	fi
}

case "$1" in
--toolchain)
	QUIET=1
	toolchain
	[ "$missing" = 0 ] || { echo "Install the above, then run make again."; exit 1; }
	;;
--quiet)
	QUIET=1
	runtime
	[ "$missing" = 0 ] || echo "The app fetches what it needs on its first run. make check lists everything."
	;;
*)
	echo "Building"
	toolchain
	echo "Running"
	runtime
	[ "$missing" = 0 ] && echo "Everything is in place." || exit 1
	;;
esac
exit 0
