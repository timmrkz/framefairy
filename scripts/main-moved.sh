#!/bin/sh
# Tells every open pull request main no longer merges into, with a
# comment, and says which files conflict.
#
#   sh scripts/main-moved.sh        # every open pull request
#   sh scripts/main-moved.sh 180    # pull request #180 alone
#
# A comment on a pull request is what reaches the Claude session watching
# it. A push to main is not: the session is subscribed to its own pull
# request, and nothing on it changes when main does. That is how a pull
# request sat with conflicts nobody knew about. So this runs on every push
# to main, from .github/workflows/main-moved.yml, and speaks up on each
# pull request main no longer merges into. One that still merges is left
# alone: merging main into it would only add a commit and a build, see
# CLAUDE.md.
#
# It also runs for one pull request whenever that pull request is opened
# or pushed to. A branch started from a main that has moved on since is in
# conflict from its first push, and so is a merge of main that brought a
# conflict back, and neither is a push to main. #180 was opened on a main
# one merge behind and sat in conflict for an hour, until the next merge
# to main told it.
#
# It merges in git and does not ask GitHub whether the pull request is
# mergeable, because GitHub works that out some time after a push and
# answers that it does not know yet in the meantime.
#
# It only reads the branches. Merging main in, resolving what conflicts
# and testing the result is the work of whoever drives the pull request,
# because a merge made here would be pushed without anyone running make
# test on it.
#
# Run from a checkout of main with the whole history, with gh signed in.
# scripts/main-moved-test.sh checks it.
set -eu

only=${1-}
case $only in
'' | *[!0-9]*) [ -z "$only" ] || {
	echo "main-moved: $only is not a pull request number" >&2
	exit 2
} ;;
esac

main=$(git rev-parse HEAD)
short=$(git rev-parse --short HEAD)
subject=$(git log -1 --format=%s HEAD)

# Pull requests into main from branches of this repository. A fork's branch
# is not ours to fetch and speak for.
gh pr list --base main --state open --limit 100 \
	--json number,headRefName,isCrossRepository \
	--jq '.[] | "\(.number) \(.headRefName) \(.isCrossRepository)"' |
	while read -r number branch cross; do
		[ "$cross" = false ] || continue
		[ -z "$only" ] || [ "$number" = "$only" ] || continue
		if ! git fetch -q origin "+refs/heads/$branch:refs/remotes/origin/$branch"; then
			echo "#$number: could not fetch $branch" >&2
			continue
		fi
		head=$(git rev-parse --short "origin/$branch")
		if git merge-base --is-ancestor "$main" "origin/$branch"; then
			echo "#$number: main is already in $branch"
			continue
		fi
		# Exit 0 merges cleanly, 1 conflicts, and anything else is git
		# failing, which is said as that and not taken for either.
		status=0
		out=$(git merge-tree --write-tree --name-only --no-messages "origin/$branch" "$main") || status=$?
		case $status in
		0)
			echo "#$number: merges cleanly"
			continue
			;;
		1) conflicts=$(printf '%s\n' "$out" | sed 1d) ;;
		*)
			echo "#$number: could not merge main into $branch" >&2
			continue
			;;
		esac
		echo "#$number: conflicts in $(printf '%s' "$conflicts" | tr '\n' ' ')"

		# One comment per pull request for each pair of main and the
		# branch, even when the workflow runs twice for the same push. A
		# branch that merged main and then fell into conflict with the
		# same main again is a new pair, and is told again.
		mark="<!-- main-moved $short $head -->"
		if gh pr view "$number" --json comments --jq '.comments[].body' | grep -qF "$mark"; then
			echo "#$number: already told about $short at $head"
			continue
		fi
		list=$(printf '%s\n' "$conflicts" | sed 's/.*/- `&`/')
		body=$(printf '%s\n\n%s\n\n%s\n\n%s' \
			"main is at $short, $subject. It no longer merges into this branch at $head. These files conflict:" \
			"$list" \
			"Merge main into this branch, resolve them, read CLAUDE.md again, because main may have changed the rules, then run the tests of what the resolution touches, push, and run \`make changed\` after the push." \
			"$mark")
		gh pr comment "$number" --body "$body"
	done
