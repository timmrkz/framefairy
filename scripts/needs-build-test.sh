#!/bin/sh
# Checks the rule in needs-build.sh, because a mistake in it is silent: a
# commit gets no build and the Updates page goes on offering the one
# before, or it says a build is coming that never will. Every case below is
# one a push really arrives as.
#
#   sh scripts/needs-build-test.sh
#
# It makes a small repository of its own with the commits it needs, so
# nothing is fetched and nothing is asked of GitHub.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

cd "$work"
git init -q
git config user.email test@example.com
git config user.name test
git config commit.gpgsign false

commit() {
	git add -A
	git commit -q --allow-empty -m "$1"
	git rev-parse HEAD
}

mkdir -p engine docs
echo a >engine/run.go
app=$(commit app)
echo b >docs/APP.md
docs=$(commit docs)
echo c >README.md
readme=$(commit readme)
echo d >engine/run.go
code=$(commit code)
empty=$(commit nothing)
# A change taken back: the files are the ones built, whatever happened
# on the way.
echo a >engine/run.go
back=$(commit back)

check() {
	what=$1
	built=$2
	newest=$3
	want=$4
	got=$(sh "$here/needs-build.sh" "$built" "$newest" 2>/dev/null)
	if [ "$got" = "$want" ]; then
		printf 'ok  \t%-44s %s\n' "$what" "$got"
	else
		printf 'FAIL\t%-44s %s, wanted %s\n' "$what" "$got" "$want"
		fail=1
	fi
}

check "a channel with no build" "" "$app" build
check "the commit that is built" "$app" "$app" built
check "the built commit, shortened" "$(echo "$app" | cut -c1-12)" "$app" built
check "a push of the docs" "$app" "$docs" docs
check "a README is docs too" "$app" "$readme" docs
check "two pushes of docs, one after the other" "$docs" "$readme" docs
check "code after docs" "$app" "$code" build
check "code, measured from what is built" "$docs" "$code" build
check "nothing changed in the commit" "$code" "$empty" built
check "code changed and changed back" "$app" "$back" docs
check "a commit nobody can read" "$app" "0123456789abcdef0123456789abcdef01234567" build
check "a build nobody can read" "0123456789ab" "$code" build

exit $fail
