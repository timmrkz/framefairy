#!/bin/sh
# Checks scripts/blob-times.py on what the job linux of ci.yml does with
# it, because a mistake there is silent: Go reuses a test result for a
# file that changed, and a pull request goes green on a test that fails.
#
#   sh scripts/blob-times-test.sh
#
# A test reads cases.json. A pull request changes one value in it, main
# changes another line of it, and CI checks out the merge of the two, as
# GitHub does on a pull request. With the times from the history the merge
# kept main's size and time, and Go said (cached) to a test that failed,
# see #457. With the times from the content it must run, and on main again
# it must be (cached). It needs Go and no network.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export GOCACHE="$tmp/gocache" GOFLAGS= GOTOOLCHAIN=local
fail=0

cd "$tmp" && mkdir repo && cd repo
git init -q -b main
git config user.email ci@example.com
git config user.name ci
cp "$here/testdata/blob-times/go.mod" go.mod
cp "$here/testdata/blob-times/cases_test.txt" cases_test.go
printf 'a=0.25\nb=1\nc=x\n' >cases.json
git add . && git commit -qm base
git checkout -qb pr && sed -i.bak 's/a=0.25/a=0.75/' cases.json && rm cases.json.bak && git commit -qam pr
git checkout -q main && sed -i.bak 's/c=x/c=y/' cases.json && rm cases.json.bak && git commit -qam main

# The steps of the job: times from the content, then one time for every
# folder, then the tests.
run() {
	git ls-files -s -z | python3 -I "$here/blob-times.py"
	find . -path './.git' -prune -o -type d -exec touch -t 202601010000 {} +
	go test ./... 2>&1 || true
}

# check NAME WANT OUT: a line of what the tests said, OUT, starts with WANT.
check() {
	if printf '%s\n' "$3" | grep -q "^$2"; then
		printf 'ok  \t%s\n' "$1"
	else
		printf 'FAIL\t%s, the tests said:\n%s\n' "$1" "$3"
		fail=1
	fi
}

check "main runs its test" "ok  	sim	[0-9]" "$(run)"
git checkout -qb merge && git merge -q --no-ff pr -m merge >/dev/null
check "the merge with a changed value is tested again" "FAIL" "$(run)"
git checkout -q main
check "main again is reused" "ok  	sim	(cached)" "$(run)"

[ "$fail" = 0 ] && echo "ok  	file times come from the content"
exit $fail
