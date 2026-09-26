#!/bin/sh
# Tells every open pull request that main has moved, with a comment, and
# says whether main still merges into it and which files do not.
#
# A comment on a pull request is what reaches the Claude session watching
# it. A push to main is not: the session is subscribed to its own pull
# request, and nothing on it changes when main does. That is how a pull
# request sat with conflicts nobody knew about. So this runs on every push
# to main, from .github/workflows/main-moved.yml, and speaks up on each
# pull request main does not already sit in.
#
# It only reads the branches. Merging main in, resolving what conflicts
# and testing the result is the work of whoever drives the pull request,
# because a merge made here would be pushed without anyone running make
# test on it.
#
# Run from a checkout of main with the whole history, with gh signed in.
set -eu

main=$(git rev-parse HEAD)
short=$(git rev-parse --short HEAD)
subject=$(git log -1 --format=%s HEAD)

git config user.name "main-moved"
git config user.email "main-moved@users.noreply.github.com"

# Pull requests into main from branches of this repository. A fork's branch
# is not ours to fetch and speak for.
gh pr list --base main --state open --limit 100 \
	--json number,headRefName,isCrossRepository \
	--jq '.[] | select(.isCrossRepository | not) | "\(.number) \(.headRefName)"' |
	while read -r number branch; do
		if ! git fetch -q origin "refs/heads/$branch:refs/remotes/origin/$branch"; then
			echo "#$number: could not fetch $branch" >&2
			continue
		fi
		if git merge-base --is-ancestor "$main" "origin/$branch"; then
			echo "#$number: main is already in $branch"
			continue
		fi
		git checkout -q --detach "origin/$branch"
		if git merge -q --no-commit --no-ff "$main" >/dev/null 2>&1; then
			conflicts=""
		else
			conflicts=$(git diff --name-only --diff-filter=U)
		fi
		git merge --abort 2>/dev/null || git reset -q --hard
		git checkout -q --detach "$main"

		if [ -n "$conflicts" ]; then
			list=$(printf '%s\n' "$conflicts" | sed 's/.*/- `&`/')
			body=$(printf '%s\n\n%s\n\n%s\n\n%s' \
				"main moved to $short, $subject. It no longer merges into this branch. These files conflict:" \
				"$list" \
				"Merge main into this branch, resolve them, read CLAUDE.md again, because main may have changed the rules, then run \`make changed\` and push." \
				"<!-- main-moved $short -->")
			echo "#$number: conflicts in $(printf '%s' "$conflicts" | tr '\n' ' ')"
		else
			body=$(printf '%s\n\n%s' \
				"main moved to $short, $subject. It merges into this branch without conflicts. Merge it in, read CLAUDE.md again, because main may have changed the rules, then run \`make changed\` and push, so this pull request is tested against what it will land on." \
				"<!-- main-moved $short -->")
			echo "#$number: merges cleanly"
		fi
		# One comment per pull request per move of main, even when the
		# workflow runs twice for the same push.
		if gh pr view "$number" --json comments --jq '.comments[].body' | grep -qF "<!-- main-moved $short -->"; then
			echo "#$number: already told about $short"
			continue
		fi
		gh pr comment "$number" --body "$body"
	done
