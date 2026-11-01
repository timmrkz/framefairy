#!/bin/sh
# Prints the Go release a Mac job of CI sets up for the go line of
# go.mod, worked out the way actions/setup-go works it out: the newest
# stable release in its versions manifest that the line names, 1.27.3 for
# go 1.27.
#
#   sh scripts/go-version.sh [MANIFEST]
#
# MANIFEST is a copy of the manifest, for scripts/go-version-test.sh.
# Without it the manifest is fetched.
#
# With the release known before setup-go runs, a job can restore that
# release into the runner's tool cache from a cache of its own, and
# setup-go finds it there. Otherwise setup-go downloads Go and then spends
# 11 s of a 20 s setup copying it into a tool cache the runner throws away,
# #408.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
want=$(awk '$1 == "go" { print $2; exit }' "$here/../go.mod")
[ -n "$want" ] || { echo "go.mod has no go line" >&2; exit 1; }

if [ $# -gt 0 ]; then
	manifest=$(cat "$1")
else
	manifest=$(curl -fsSL --retry 3 https://raw.githubusercontent.com/actions/go-versions/main/versions-manifest.json)
fi

# The manifest lists the newest first, as setup-go reads it.
version=$(printf '%s' "$manifest" | jq -r --arg want "$want" \
	'[.[] | select(.stable and (.version == $want or (.version | startswith($want + "."))))][0].version // empty')
[ -n "$version" ] || { echo "no stable Go release in the manifest for go $want" >&2; exit 1; }
echo "$version"
