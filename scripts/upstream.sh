#!/bin/sh
# Says, for each program and library we build from source and ship, the
# version pinned here and the newest release upstream, and whether we are
# behind.
#
#   scripts/upstream.sh          the whole table
#   scripts/upstream.sh --behind only what is behind, and exit 1 if anything is
#
# Dependabot watches the Go modules and the interface's packages. Nothing
# watches what the build scripts fetch, and ffmpeg is what reads a video
# somebody else made, so this does: .github/workflows/security.yml runs it
# every week and keeps one issue open while anything is behind.
#
# llama.cpp makes a build a day, so it counts as behind only once it is
# LLAMA_SLACK builds back, about a month.
set -e
LLAMA_SLACK=${LLAMA_SLACK:-30}
behind_only=0
[ "${1:-}" = "--behind" ] && behind_only=1

pinned() { awk -F= -v k="$1" '$1 == k { print $2; exit }' "$2"; }

# The newest tag of a repository that matches a pattern, with what the
# pattern does not cover taken off: release candidates, dev tags and the
# like.
newest() {
	git ls-remote --tags --refs "$1" 2>/dev/null | awk '{ sub("refs/tags/", "", $2); print $2 }' |
		grep -E "$2" | sed -E "s/$3//" | sort -V | tail -1
}

ffmpeg=$(pinned FFMPEG_VERSION scripts/build-ffmpeg.sh)
freetype=$(pinned FREETYPE_VERSION scripts/build-ffmpeg.sh)
fribidi=$(pinned FRIBIDI_VERSION scripts/build-ffmpeg.sh)
harfbuzz=$(pinned HARFBUZZ_VERSION scripts/build-ffmpeg.sh)
libass=$(pinned LIBASS_VERSION scripts/build-ffmpeg.sh)
llama=$(pinned LLAMA_VERSION scripts/build-llama.sh)
sherpa=$(awk '/k2-fsa\/sherpa-onnx-go /{ print $2; exit }' go.mod | sed 's/^v//')

out=""
any=0
row() {
	name=$1 have=$2 want=$3 late=$4
	if [ -z "$want" ]; then
		state="upstream did not answer"
	elif [ "$late" = 1 ]; then
		state="behind"
		any=1
	else
		state="current"
	fi
	if [ "$behind_only" = 0 ] || [ "$state" != current ]; then
		out="$out$(printf '| %-9s | %-9s | %-9s | %-23s |' "$name" "$have" "$want" "$state")
"
	fi
}
later() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$2" ]; }
check() {
	if [ -n "$3" ] && later "$2" "$3"; then row "$1" "$2" "$3" 1; else row "$1" "$2" "$3" 0; fi
}

check ffmpeg "$ffmpeg" "$(newest https://github.com/FFmpeg/FFmpeg '^n[0-9]+(\.[0-9]+)+$' '^n')"
check freetype "$freetype" "$(newest https://github.com/freetype/freetype '^VER-[0-9]+-[0-9]+-[0-9]+$' '^VER-' | tr - .)"
check fribidi "$fribidi" "$(newest https://github.com/fribidi/fribidi '^v[0-9]+(\.[0-9]+)+$' '^v')"
check harfbuzz "$harfbuzz" "$(newest https://github.com/harfbuzz/harfbuzz '^[0-9]+(\.[0-9]+)+$' '^')"
check libass "$libass" "$(newest https://github.com/libass/libass '^[0-9]+(\.[0-9]+)+$' '^')"
check sherpa "$sherpa" "$(newest https://github.com/k2-fsa/sherpa-onnx '^v[0-9]+(\.[0-9]+)+$' '^v')"
latest=$(newest https://github.com/ggml-org/llama.cpp '^b[0-9]+$' '^b')
if [ -n "$latest" ] && [ "$((latest - ${llama#b}))" -ge "$LLAMA_SLACK" ]; then
	row llama.cpp "$llama" "b$latest" 1
else
	row llama.cpp "$llama" "${latest:+b$latest}" 0
fi

if [ -n "$out" ]; then
	printf '| %-9s | %-9s | %-9s | %-23s |\n' what pinned newest ""
	printf '| %-9s | %-9s | %-9s | %-23s |\n' --------- --------- --------- -----------------------
	printf '%s' "$out"
fi
if [ "$behind_only" = 1 ] && [ "$any" = 1 ]; then
	exit 1
fi
exit 0
