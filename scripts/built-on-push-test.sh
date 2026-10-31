#!/bin/sh
# Checks the rule in built-on-push.sh, because a mistake in it is silent: a
# pull request opened gets no build, and the Updates page has nothing to
# offer for it. Every case below is one a pull request really opens as.
#
#   sh scripts/built-on-push-test.sh
#
# It stands in for gh with a script that prints the runs a case gives it,
# so nothing is asked of GitHub.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

mkdir "$work/bin"
cat >"$work/bin/gh" <<'GH'
#!/bin/sh
# run list: the runs in $GH_DIR/runs, or a failure when there is none.
case "$1 $2" in
"run list") cat "$GH_DIR/runs" ;;
*) echo "gh: $*" >&2 && exit 1 ;;
esac
GH
chmod +x "$work/bin/gh"
export GH_DIR="$work" PATH="$work/bin:$PATH"

check() {
	what=$1
	runs=$2
	want=$3
	if [ -n "$runs" ]; then printf '%s\n' "$runs" >"$work/runs"; else rm -f "$work/runs"; fi
	got=$(sh "$here/built-on-push.sh" 2026-10-31T13:20:20Z 36033381ba4c 2>/dev/null)
	if [ "$got" = "$want" ]; then
		printf 'ok  \t%-52s %s\n' "$what" "$got"
	else
		printf 'FAIL\t%-52s %s, wanted %s\n' "$what" "$got" "$want"
		fail=1
	fi
}

# Dependabot's #640: pushed and opened in the same second, the push's run
# made a second after the opening.
check "a push made after the opening" '[{"createdAt":"2026-10-31T13:20:21Z"}]' push
check "a push made in the second of the opening" '[{"createdAt":"2026-10-31T13:20:20Z"}]' push
# A session pushes its branch and opens the pull request later.
check "a branch pushed before it was opened" '[{"createdAt":"2026-10-31T12:58:02Z"}]' build
check "pushed before, and once more after" '[{"createdAt":"2026-10-31T12:58:02Z"},{"createdAt":"2026-10-31T13:20:30Z"}]' push
check "no push of the commit" '[]' build
check "runs that cannot be read" "" build
check "runs that do not read" 'not json' build

exit "$fail"
