#!/bin/sh
# Checks that every action a workflow uses is pinned to a commit, with the
# full version that commit is beside it.
#
#   sh scripts/pins.sh
#
# A tag like v7 can be moved to other code, a commit cannot, so the commit
# is what runs. The comment says which release that commit is, and it says
# it in full, v7.0.2 and never v7: v7 is a line of releases, so a bump
# from one v7 to the next read as the same v7 with a new commit, which
# looks like the pin was swapped. Dependabot moves the full version in the
# comment together with the commit, see docs/BUILD.md, Keeping up. A
# release whose own tags have two numbers, like v2.3 of
# chetan/git-restore-mtime-action, is named in full with two.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fail=0

for f in "$here"/../.github/workflows/*.yml; do
	# Every line that uses something not in this repository. A local
	# action, ./ or a docker:// image, has no release to name.
	bad=$(grep -nE '^[[:space:]-]*uses:' "$f" | grep -vE 'uses:[[:space:]]*(\./|docker://)' |
		grep -vE 'uses:[[:space:]]*[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}[[:space:]]+# v[0-9]+\.[0-9]+(\.[0-9]+)?[[:space:]]*$' || true)
	[ -z "$bad" ] && continue
	echo "$bad" | while IFS= read -r line; do
		echo "FAIL: $(basename "$f"):$line" >&2
	done
	fail=1
done

if [ "$fail" = 1 ]; then
	echo "      pin each to a commit and name its full version, like @<commit> # v7.0.2" >&2
	exit 1
fi
printf 'ok  \taction pins\n'
