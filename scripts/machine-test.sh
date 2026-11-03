#!/bin/sh
# Checks the table machine.sh gives of each package's time, because a
# mistake in it is silent: the table only goes missing from the log, or
# reads a failed package as a slow one, #592.
#
#   sh scripts/machine-test.sh
#
# testdata/machine/tests.json holds go test -json events the way
# gotestsum keeps them, with a package that was cached, one that passed
# in 101.21 s, one that failed at the timeout, one under a second, one
# skipped, the events of single tests, and a test's output that quotes a
# package's event, none of which is a package that ended.
# tests.want is the table for a fixed work of 1.5 s.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data=$here/testdata/machine
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

sh "$here/machine.sh" packages 1.5 "$data/tests.json" >"$work/got"
if ! cmp -s "$data/tests.want" "$work/got"; then
	printf 'FAIL\tmachine.sh packages\n'
	diff "$data/tests.want" "$work/got" | sed 's/^/\t/' || true
	exit 1
fi

# Nothing to read gives no table and no failure.
sh "$here/machine.sh" packages 1.5 "$work/none.json" >"$work/got"
if [ -s "$work/got" ]; then
	printf 'FAIL\tmachine.sh packages printed for a file that is not there\n'
	exit 1
fi
printf 'ok  \tmachine.sh packages\n'
