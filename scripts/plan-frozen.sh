#!/bin/sh
# Fails when a change edits docs/GUI-PLAN.md after it was frozen.
#
#   sh scripts/plan-frozen.sh [BASE]
#
# Every pull request used to add its row at the end of the same table in
# the plan, so any two of them conflicted, however they were numbered.
# Open work is GitHub issues now, which live in no file, and the plan is
# history. This keeps it so: a pull request that changes a line of the
# plan is told to open an issue instead.
#
# BASE is what the change is compared with. CI hands it nothing and it
# takes HEAD^1, the base branch of the merge commit a pull request is
# checked out as. By hand it takes where the branch left origin/main. A
# base whose plan does not say it is frozen yet passes, which is the pull
# request that froze it.
set -eu

plan=docs/GUI-PLAN.md
marker='<!-- frozen: open work is in the GitHub issues'

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

base=${1:-}
if [ -z "$base" ]; then
	if [ -n "${GITHUB_ACTIONS:-}" ]; then
		base=HEAD^1
	else
		base=$(git merge-base HEAD origin/main 2>/dev/null || git merge-base HEAD main)
	fi
fi

if ! git show "$base:$plan" 2>/dev/null | grep -qF "$marker"; then
	echo "ok  	$plan is not frozen on $base yet"
	exit 0
fi

# The working tree, so make lint sees an edit that is not committed.
if git diff --quiet "$base" -- "$plan"; then
	echo "ok  	$plan is unchanged"
	exit 0
fi

cat >&2 <<MSG
FAIL	$plan is history and this change edits it:

$(git diff --stat "$base" -- "$plan")

Open work is GitHub issues now, see CLAUDE.md. Take the edit out with

	git checkout $base -- $plan

and open an issue for each row it added, with what was seen, where and
who found it. A pull request that finishes an issue says Fixes #N.
MSG
exit 1
