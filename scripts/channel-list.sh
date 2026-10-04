#!/bin/sh
# Writes the channel list, channels.json, from every channel's entry in the
# release called dev, and uploads it. The publish workflow runs it in two
# places: once a build is published, and the moment a push to a pull
# request starts, so the list says a newer commit is on its way for the
# whole of its build and not only once the build is done. It always runs
# from main. See .github/workflows/publish.yml and docs/UPDATES.md.
#
#   sh scripts/channel-list.sh
#
# It needs gh, jq, Go and a checkout of this repository, with GH_TOKEN and
# GH_REPO set. It leaves channels.json in the folder it runs in.
set -eu

entries=$(mktemp -d)
trap 'rm -rf "$entries"' EXIT
gh release download dev --pattern 'channel-*.json' --dir "$entries" || true

# Every open pull request with its newest commit, one a line.
newest=""
if open=$(gh pr list --state open --limit 1000 --json number,headRefOid \
	--jq '.[] | "\(.number) \(.headRefOid)"'); then
	for entry in "$entries"/channel-pr-*.json; do
		[ -e "$entry" ] || continue
		number=${entry##*/channel-pr-}
		number=${number%.json}
		head=$(printf '%s\n' "$open" | awk -v n="$number" '$1 == n { print $2 }')
		# The entry of a pull request that is not open any more goes. The
		# close takes it away by itself, but GitHub runs nothing for a pull
		# request that no longer merges into main, so one closed in that
		# state kept its entry for good, and the app went on offering it:
		# #24. Only when the list of open pull requests could be read, or a
		# failure to read it would take every entry away.
		if [ -z "$head" ]; then
			echo "taking away the entry of pull request $number, which is not open"
			gh release delete-asset dev "channel-pr-$number.json" -y || true
			rm -f "$entry"
			continue
		fi
		# A newer commit than the one built, with a build to come, is said
		# beside the build, by the same rule the build goes by. One that
		# only changes the docs has no build coming, and a list that said
		# it had would say so for good.
		built=$(jq -r '.commit // empty' "$entry")
		if [ "$(sh scripts/needs-build.sh "$built" "$head")" = build ]; then
			echo "pull request $number has $head still to be built"
			newest="$newest -newest pr-$number=$(echo "$head" | cut -c1-12)"
		fi
	done
fi

set -- "$entries"/*.json
[ -e "$1" ] || set --
# $newest is words the loop above made of a number and a commit, so it is
# split on purpose.
# shellcheck disable=SC2086
go run ./cmd/framefairy-release list -out channels.json $newest "$@"
cat channels.json

# The new list goes up under a name of its own and is then renamed into
# place. GitHub cannot replace a release file in one step, and gh release
# upload --clobber deletes the old one before it uploads the new, so every
# app that checked during the upload was answered 404. Now the list is
# missing only between the delete and the rename, two calls, and the app
# waits out a 404 anyway, see Fetch in updates/source.go. If the rename
# fails the list is uploaded the old way, so it is never left missing.
cp channels.json channels.next.json
gh release upload dev channels.next.json --clobber
next=$(gh api 'repos/{owner}/{repo}/releases/tags/dev' \
	--jq '.assets[] | select(.name == "channels.next.json") | .id')
gh release delete-asset dev channels.json -y || true
if ! gh api -X PATCH "repos/{owner}/{repo}/releases/assets/$next" -f name=channels.json >/dev/null; then
	echo "renaming the list failed, uploading it in place instead"
	gh release upload dev channels.json --clobber
	gh release delete-asset dev channels.next.json -y || true
fi

# Two copies of the list, channels-a.json and channels-b.json, which the
# apps read in place of channels.json, keeping the one written last, see
# ListCopies in updates/updates.go. Only the older copy is replaced, so
# the newer one stays where it is the whole time. channels.json above is
# still missing for a moment each time, and for as long again as GitHub
# takes to serve the new file, a few seconds, and Tim's app ran into that
# with seven seconds of waiting. The workflow writes the list no more than
# once at a time and every writing takes half a minute, so the copy left
# alone is never one GitHub is still catching up on. channels.json stays
# for the builds made before the copies.
copy=$(gh api 'repos/{owner}/{repo}/releases/tags/dev' --jq '
	[.assets[] | {name, updated_at}] as $have
	| ["channels-a.json", "channels-b.json"]
	| map(. as $n | {name: $n, at: ([$have[] | select(.name == $n) | .updated_at] | first // "")})
	| sort_by(.at) | .[0].name')
echo "replacing $copy, the older copy of the list"
cp channels.json "$copy"
gh release delete-asset dev "$copy" -y || true
gh release upload dev "$copy" --clobber
