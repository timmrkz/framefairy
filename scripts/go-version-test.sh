#!/bin/sh
# Checks scripts/go-version.sh against a manifest of its own, so nothing
# is fetched. A wrong answer would make the Mac jobs of CI set up another
# Go than the line in go.mod names, or none.
#
#   sh scripts/go-version-test.sh
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

want=$(awk '$1 == "go" { print $2; exit }' "$here/../go.mod")
major=${want%.*}
minor=${want#*.}
minor=${minor%%.*}
next="$major.$((minor + 1))"

# Newest first, as the real manifest is: a release candidate of the next
# Go, a newer one of the next, then the line's own releases, newest
# first, with an unstable one on top, and a release whose number only
# starts like the line's.
cat >"$work/manifest.json" <<EOF
[
  {"version": "$next.0-rc1", "stable": false},
  {"version": "$next.1", "stable": true},
  {"version": "$want.9-rc1", "stable": false},
  {"version": "$want.3", "stable": true},
  {"version": "$want.2", "stable": true},
  {"version": "${want}0.1", "stable": true}
]
EOF

got=$(sh "$here/go-version.sh" "$work/manifest.json")
if [ "$got" = "$want.3" ]; then
	echo "ok	go $want sets up $got"
else
	echo "FAIL	go $want sets up $got, expected $want.3"
	fail=1
fi

echo '[{"version": "'"$next"'.1", "stable": true}]' >"$work/none.json"
if sh "$here/go-version.sh" "$work/none.json" >/dev/null 2>&1; then
	echo "FAIL	a manifest with no release for go $want gave an answer"
	fail=1
else
	echo "ok	a manifest with no release for go $want fails"
fi

exit "$fail"
