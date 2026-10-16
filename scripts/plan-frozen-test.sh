#!/bin/sh
# Checks plan-frozen.sh on a repository of its own, because a mistake in it
# is silent: rows creep back into the plan, and with them the conflicts.
#
#   sh scripts/plan-frozen-test.sh
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
unset GITHUB_ACTIONS

repo=$work/repo
mkdir -p "$repo/scripts" "$repo/docs"
cp "$here/plan-frozen.sh" "$repo/scripts/"
cd "$repo"
git init -q -b main
printf '# GUI plan\n\n| 2.1 | a row | `[ ]` |\n' >docs/GUI-PLAN.md
git add -A
git commit -qm before

# expect WANT NAME BASE: the check passes (0) or fails (1) against BASE.
expect() {
	want=$1 name=$2 base=$3
	if sh scripts/plan-frozen.sh "$base" >"$work/out" 2>&1; then got=0; else got=1; fi
	if [ "$got" = "$want" ]; then
		printf 'ok  \t%s\n' "$name"
	else
		printf 'FAIL\t%s: wanted %s, got %s\n' "$name" "$want" "$got"
		sed 's/^/\t/' "$work/out"
		fail=1
	fi
}

# The pull request that freezes the plan edits it, against a base that
# does not say it is frozen yet.
git checkout -qb freeze
printf '# GUI plan\n\n<!-- frozen: open work is in the GitHub issues -->\n\n| 2.1 | a row | `[ ]` |\n' >docs/GUI-PLAN.md
git commit -qam freeze
expect 0 "the pull request that freezes the plan" main
git checkout -q main
git merge -q --ff-only freeze

git checkout -qb work
echo code >app.go
git add app.go
git commit -qm code
expect 0 "a change that leaves the plan alone" main

echo '| 2.2 | a new row | `[ ]` |' >>docs/GUI-PLAN.md
expect 1 "a row added and not committed" main
git commit -qam row
expect 1 "a row added in a commit" main

git checkout -q main -- docs/GUI-PLAN.md
git commit -qm "row taken out"
expect 0 "the row taken out again" main

sed -i.bak 's/`\[ \]`/`[x]`/' docs/GUI-PLAN.md && rm docs/GUI-PLAN.md.bak
expect 1 "a status changed" main
git checkout -q -- docs/GUI-PLAN.md

# CI checks a pull request out as a merge commit and hands no base, so
# the base is HEAD^1.
git checkout -qb ci main
echo '| 2.3 | another row | `[ ]` |' >>docs/GUI-PLAN.md
git commit -qam "row in a pull request"
git checkout -q main
git merge -q --no-ff -m merge ci
if GITHUB_ACTIONS=true sh scripts/plan-frozen.sh >"$work/out" 2>&1; then
	printf 'FAIL\ta row in the merge commit CI checks out passed\n'
	fail=1
else
	printf 'ok  \ta row in the merge commit CI checks out\n'
fi

exit $fail
