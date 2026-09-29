#!/bin/sh
# Says whether a pull request's commit needs a build of its own, given the
# commit its channel's build was made from. The build workflow asks it
# before building, and the list asks it to know whether a pull request has
# a commit still to come. One rule in one place, so the two never disagree
# about a commit: the list said a build was coming that the build had
# decided against, and the Updates page waited for it for good.
#
#   sh scripts/needs-build.sh BUILT NEWEST
#
# It writes one word on standard output and says on standard error why:
#
#   build   NEWEST changes the app, or what it changes cannot be read
#   built   NEWEST is the commit built, or has the same files
#   docs    NEWEST only changes the docs against what is built
#
# BUILT may be empty, a channel with no build yet, and may be shortened,
# the way the channel list keeps it. Both are fetched by themselves when
# the checkout does not have them, one commit deep, since comparing two
# commits' files needs nothing of their history.
#
# It compares against what is built and not against the push before, which
# is what the workflow once did. A push that stops the build of the one
# before it and only changes the docs would otherwise leave the one before
# with no build at all.
#
# Every doubt answers build. A build too many costs a few minutes of a
# runner, a build too few is a commit Tim tests without having it.
set -u

built=${1-}
newest=${2:?say which commit to decide about}

say() { echo "$*" >&2; }
answer() {
	say "$2"
	echo "$1"
	exit 0
}

[ -n "$built" ] || answer build "The channel has no build yet."

# A commit the checkout has, or one fetched. The channel list keeps twelve
# characters of a commit, which git cannot fetch, so a short one is asked
# of GitHub first.
full() {
	if sha=$(git rev-parse -q --verify "$1^{commit}" 2>/dev/null); then
		echo "$sha"
		return 0
	fi
	sha=$1
	if [ "${#sha}" -lt 40 ]; then
		sha=$(gh api "repos/${GH_REPO:-${GITHUB_REPOSITORY:-}}/commits/$sha" --jq .sha 2>/dev/null) || return 1
	fi
	git fetch -q --depth=1 origin "$sha" 2>/dev/null || return 1
	git rev-parse -q --verify "$sha^{commit}"
}

from=$(full "$built") || answer build "The commit built, $built, cannot be read."
to=$(full "$newest") || answer build "The commit $newest cannot be read."
changed=$(git diff --name-only "$from" "$to") || answer build "What changed since $built cannot be read."
[ -n "$changed" ] || answer built "$newest has the same files as the build of $built."
echo "$changed" | grep -qvE '^docs/|\.md$' || answer docs "$newest only changes the docs against the build of $built."
answer build "$newest changes the app against the build of $built."
