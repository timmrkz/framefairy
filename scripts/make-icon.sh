#!/bin/sh
# Builds the app icon from one PNG.
#
#   scripts/make-icon.sh [OUT]
#
# The source is build/icon.png, one square 1024 by 1024 image with
# transparency, and it is the only file anybody ever has to change. What
# comes out is an .icns, which is not an image but a container holding the
# same artwork at ten sizes, from 16 to 1024. macOS picks which one the
# Dock, Finder, Spotlight and the switcher each get, so drawing one and
# letting it be scaled is not the same thing at all.
#
# Both tools are on every Mac already. sips resizes, iconutil packs. The
# .icns is build output and is not in the repository.
#
# About the grid, because it decides whether the icon looks right beside
# the system's own. A macOS app icon's rounded square fills about 824 of
# the 1024, centred, with the rest left for the shadow the system draws.
# build/icon.png is on that grid. build/icon-full-bleed.png is the same
# artwork filling the whole square, kept because macOS 26 may mask a
# legacy .icns into its own shape, and if it does, the full-bleed one is
# the right input. That is one look on a Mac rather than a guess, and the
# swap is copying one file over the other.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SRC="$ROOT/build/icon.png"
OUT=${1:-$ROOT/.build/icon.icns}

if [ ! -f "$SRC" ]; then
	echo "make-icon.sh: no $SRC, so there is no icon to build." >&2
	exit 1
fi
if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "make-icon.sh: an .icns is made with sips and iconutil, which are macOS." >&2
	exit 1
fi

# Nothing to do when the icon is older than nothing. An icon is not rebuilt
# on every make, because make has to stay quick when there is no work.
if [ -f "$OUT" ] && [ "$OUT" -nt "$SRC" ] && [ "$OUT" -nt "$0" ]; then
	exit 0
fi

WORK=$(dirname "$OUT")/icon.iconset
rm -rf "$WORK"
mkdir -p "$WORK" "$(dirname "$OUT")"

# The ten that macOS asks for. The names are fixed, iconutil reads them.
# 16 and 32 are drawn twice over, once for a normal screen and once for a
# retina one, which is why 32 and 64 appear in two rows.
set -- \
	"16 icon_16x16.png" \
	"32 icon_16x16@2x.png" \
	"32 icon_32x32.png" \
	"64 icon_32x32@2x.png" \
	"128 icon_128x128.png" \
	"256 icon_128x128@2x.png" \
	"256 icon_256x256.png" \
	"512 icon_256x256@2x.png" \
	"512 icon_512x512.png" \
	"1024 icon_512x512@2x.png"
for pair in "$@"; do
	px=${pair%% *}
	name=${pair#* }
	sips -s format png -z "$px" "$px" "$SRC" --out "$WORK/$name" >/dev/null 2>&1 || {
		echo "make-icon.sh: sips could not make $name at $px px" >&2
		exit 1
	}
done

iconutil -c icns "$WORK" -o "$OUT"
rm -rf "$WORK"
echo "  icon: $OUT, from build/icon.png"
