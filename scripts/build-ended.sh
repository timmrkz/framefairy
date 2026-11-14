#!/bin/sh
# Says whether the build of a commit has ended without a build to update
# to, so the channel list stops saying one is on its way. The list says
# a commit still to be built is coming, and the Updates page then says
# "A newer commit is being built" and checks for it every 20 seconds. A
# build that failed, was cancelled or was refused by the signer never
# comes, and the page said it was coming until the next push.
#
#   sh scripts/build-ended.sh COMMIT
#
# It writes one word on standard output and says on standard error why:
#
#   ended    the commit's build failed or was cancelled, or the signer
#            refused it: nothing is coming until the next push
#   coming   the commit's build has not ended, or ended well and is being
#            published, or what became of it cannot be read
#
# It needs gh, with GH_TOKEN and GH_REPO set, and the token needs to read
# the repository's actions and commit statuses.
#
# Every doubt answers coming, the way it was before: a page that waits
# for a build too long is the old failure, a page that never says one is
# coming is a new one.
set -u

commit=${1:?say which commit to decide about}
repo=${GH_REPO:-${GITHUB_REPOSITORY:-}}

say() { echo "$*" >&2; }
answer() {
	say "$2"
	echo "$1"
	exit 0
}

# The signer's refusal is set on the commit by publish.yml, as a failed
# Ready to update to.
ready=$(gh api "repos/$repo/commits/$commit/status" \
	--jq '[.statuses[] | select(.context == "Ready to update to") | .state][0] // ""') ||
	answer coming "The statuses of $commit cannot be read."
[ "$ready" != failure ] || answer ended "The build of $commit was refused by the signer."

# The newest run of builds.yml for the commit, since a run can be run
# again, and GitHub lists the newest first.
run=$(gh api "repos/$repo/actions/workflows/builds.yml/runs?head_sha=$commit&event=push&per_page=1" \
	--jq '.workflow_runs[0] // {} | "\(.status) \(.conclusion)"') ||
	answer coming "The builds of $commit cannot be read."
case $run in
"null null" | "") answer coming "$commit has no build yet." ;;
"completed success") answer coming "The build of $commit ended well and is being published." ;;
completed\ *) answer ended "The build of $commit ended as ${run#completed }." ;;
*) answer coming "The build of $commit is ${run% *}." ;;
esac
