#!/bin/sh
# Writes engine/hyphenation again, the patterns that break a word too long
# for a caption line. Nothing in that folder is written by hand.
#
#   hyphenation.sh
#
# Two sources, each pinned to a commit:
#
# - hyph-utf8, the patterns TeX, LibreOffice and Firefox break words with,
#   for the languages below. Each language is two files: the patterns, and
#   the top of the file they came from, which names the makers, the
#   licence and the fewest letters either side of a hyphen.
# - the German word list of the Trennmuster team, who also make the German
#   patterns above. It marks where the parts of a compound join, and its
#   own build learns patterns from it that break a word only there, the
#   joints: Suchmaschinen-optimierung rather than Suchmaschinenopti-mierung.
#   Their build is run with W=1, the joints of the highest rank, which
#   their documentation names for ragged text. It needs patgen, from
#   texlive-binaries on Linux or texlive on macOS, and takes a few minutes.
#
# The languages are the ones the speech model hears, less those whose
# patterns a paid app cannot ship: Czech is under the GPL alone, Latvian
# under the LGPL or the GPL, Romanian has no licence, and Finnish grants
# only that the file may be "freely distributed", which two readings found
# unclear. Maltese has no patterns. make notices checks every licence again
# and stops at one the app may not ship. See docs/ENGINE.md.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=$ROOT/engine/hyphenation

HYPH_UTF8=https://github.com/hyphenation/tex-hyphen
HYPH_UTF8_COMMIT=5684c0f51c0b81133db2efbe60a408b4155a3ff5
WORTLISTE=https://repo.or.cz/wortliste.git
WORTLISTE_COMMIT=a68004108c475a5edf9e6502da11d6135ab9dcc1

LANGUAGES="bg da de-1996 el-monoton en-us es et fr hr hu it lt nl pl pt ru sk sl sv uk"

for tool in git curl perl make patgen; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "hyphenation: $tool is missing. patgen comes with texlive-binaries on Linux and texlive on macOS." >&2
		exit 1
	fi
done

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/out"

# The top of a TeX pattern file: its comments, up to the first line that
# is not one.
head_of() {
	awk '/^%/ { print; next } { exit }' "$1"
}

raw=https://raw.githubusercontent.com/hyphenation/tex-hyphen/$HYPH_UTF8_COMMIT/hyph-utf8/tex/generic/hyph-utf8/patterns
for lang in $LANGUAGES; do
	curl -fsSL "$raw/txt/hyph-$lang.pat.txt" -o "$WORK/out/hyph-$lang.pat.txt"
	curl -fsSL "$raw/tex/hyph-$lang.tex" -o "$WORK/hyph-$lang.tex"
	head_of "$WORK/hyph-$lang.tex" >"$WORK/out/hyph-$lang.head.txt"
done

git init -q "$WORK/wortliste"
git -C "$WORK/wortliste" fetch -q --depth 1 "$WORTLISTE" "$WORTLISTE_COMMIT"
git -C "$WORK/wortliste" checkout -q FETCH_HEAD
mkdir "$WORK/joints"
# Their build stamps the day it runs. The day of the commit it is built
# from keeps the same source giving the same file.
day=$(git -C "$WORK/wortliste" log -1 --format=%cs)
(cd "$WORK/joints" && make -s --makefile="$WORK/wortliste/Makefile" OUTDIR=. W=1 DATE="$day" major pattern-refo >build.log 2>&1) || {
	tail -20 "$WORK/joints/build.log" >&2
	exit 1
}
built=$(ls "$WORK"/joints/dehyphn-x-major/dehyphn-x-major-*.pat)
head_of "$built" >"$WORK/out/hyph-de-1996-x-major.head.txt"
# The patterns are the lines between \patterns{ and the brace that closes it.
awk '/^\\patterns\{/ { on = 1; next } on && /^\}/ { exit } on && NF { print }' "$built" \
	>"$WORK/out/hyph-de-1996-x-major.pat.txt"

rm -rf "$OUT"
mkdir -p "$OUT"
cp "$WORK"/out/* "$OUT"/
echo "ok  	hyphenation, $(ls "$OUT" | grep -c '\.pat\.txt$') pattern files in engine/hyphenation"
