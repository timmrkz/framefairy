#!/bin/sh
# Says whether a pull request just opened is built by the push of its own
# commit, so its opening can leave the build to that push. The build
# workflow asks it when a pull request is opened.
#
#   sh scripts/built-on-push.sh OPENED SHA BRANCH
#
# OPENED is when the pull request was opened, the way GitHub writes it,
# 2026-10-31T13:20:20Z. SHA is the commit it was opened on and BRANCH the
# branch it was opened from. It writes one word on standard output and
# says on standard error why:
#
#   push    a run for a push of SHA to BRANCH was made at or after the
#           opening, so it found the pull request and builds its channel
#   build   no such push, the opening builds
#
# Dependabot pushes its branch and opens its pull request in the same
# second. Both started a build of the one commit, and the second stopped
# the first, which left a cancelled build on every update it opened, #618
# and #640. A push started before the opening found no pull request and
# built nothing, so a branch pushed before it was opened is still built
# when it is opened.
#
# A push is known to have found the pull request only when its run was
# made after the opening, since the run asks for the pull request after it
# was made. A push made just before the opening may have found it too, and
# then the commit is built twice, the second stopping the first, as it was.
# Every doubt answers build: a build too many costs a few minutes of a
# runner, a build too few is a commit Tim tests without having it.
set -u

opened=${1:?say when the pull request was opened}
sha=${2:?say which commit it was opened on}
branch=${3:?say which branch it was opened from}

answer() {
	echo "$2" >&2
	echo "$1"
	exit 0
}

# gh reads the runs with the jq it carries, because the CI image has no jq
# of its own. The times are compared as text, which orders them, since
# GitHub writes every one the same way, in UTC.
stamp='^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z$'
echo "$opened" | grep -q "$stamp" || answer build "The opening, $opened, does not read as a time."
made=$(gh run list --workflow builds.yml --branch "$branch" --commit "$sha" --event push --json createdAt --jq '.[].createdAt' 2>/dev/null) ||
	answer build "The pushes of $sha cannot be read."
after=$(printf '%s\n' "$made" | awk -v opened="$opened" -v stamp="$stamp" '$0 ~ stamp && $0 >= opened { n++ } END { print n + 0 }')
[ "$after" -gt 0 ] ||
	answer build "No push of $sha started after the pull request was opened at $opened."
answer push "A push of $sha started after the pull request was opened at $opened, and builds it."
