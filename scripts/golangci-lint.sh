#!/bin/sh
# The golangci-lint make lint runs, at the version pinned here, and where
# it is. It prints the path of the program, and fetches it first when it
# is not there yet.
#
#   sh scripts/golangci-lint.sh        # the path, fetched when missing
#   INSTALL=0 sh scripts/golangci-lint.sh
#                                      # the path, or a failure that says
#                                      # how to fetch it
#
# make hands it INSTALL from the Makefile, which is 0 wherever CI is set,
# so a build runner fetches it only in a step of its own workflow, the way
# it installs everything else.
#
# The release archive from GitHub, checked against its SHA-256 below, and
# not go install or a tool line in go.mod. golangci-lint's own install
# page recommends its release builds and says the other two are not
# guaranteed to work: built here, it is a program nobody tested, and a
# tool line resolves its modules together with ours, in the one module
# framefairy is. Homebrew builds it with whatever Go it has and keeps no
# old version. The release has to be built with a Go at least as new as
# the go line of go.mod, or it refuses to read the module: 2.13.0 was the
# first release for Go 1.27, and 2.14.0 says it was built with go1.27.0.
#
# Raising the version means raising every sum beside it, from the file
# golangci-lint-VERSION-checksums.txt of the release.
set -eu

VERSION=2.14.0
SHA256_darwin_arm64=5ef5f36a7147e91dc58ef9ef4d11bb7bad5ead0c76eb6c01327a73c641d1dcc3
SHA256_darwin_amd64=a5667c1c3536be1740133213e1e822bfb8f0d98ea12903174d6d5f635e4ed68d
SHA256_linux_amd64=ab90aeb7b066f92a33415b638a50fe5344bbb75a0d32ad30cc248d88f81032ab
SHA256_linux_arm64=ee7ec5f3453d15ddf106fae5a4d6c71737712348a979d1fe9cd52ec7ea299bae

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
dir=$root/.build/golangci-lint/$VERSION
bin=$dir/golangci-lint

if [ -x "$bin" ]; then
	echo "$bin"
	exit 0
fi

if [ "${INSTALL:-1}" != 1 ]; then
	echo "golangci-lint $VERSION is not there, and make installs nothing here. Fetch it with: sh scripts/golangci-lint.sh" >&2
	exit 1
fi

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*)
	echo "golangci-lint.sh knows the releases for macOS and Linux, and this is $(uname -s)." >&2
	exit 1
	;;
esac
case $(uname -m) in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*)
	echo "golangci-lint.sh knows the releases for arm64 and amd64, and this is $(uname -m)." >&2
	exit 1
	;;
esac
eval "want=\$SHA256_${os}_${arch}"

name=golangci-lint-$VERSION-$os-$arch
url=https://github.com/golangci/golangci-lint/releases/download/v$VERSION/$name.tar.gz
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "Fetching golangci-lint $VERSION" >&2
if ! curl -sSL --fail --retry 2 --max-time 300 -o "$work/archive.tar.gz" "$url"; then
	echo "golangci-lint.sh: cannot fetch $url" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$work/archive.tar.gz" | cut -d' ' -f1)
else
	got=$(shasum -a 256 "$work/archive.tar.gz" | cut -d' ' -f1)
fi
if [ "$got" != "$want" ]; then
	echo "golangci-lint.sh: $url is not the archive pinned here." >&2
	echo "  expected sha256 $want" >&2
	echo "  got      sha256 $got" >&2
	exit 1
fi
tar -xzf "$work/archive.tar.gz" -C "$work"
mkdir -p "$dir"
mv "$work/$name/golangci-lint" "$bin.part"
mv "$bin.part" "$bin"
echo "$bin"
