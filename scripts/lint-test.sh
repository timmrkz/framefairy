#!/bin/sh
# Proves the linters that keep tests off the clock, see CLAUDE.md: each
# fails on its fixture that waits on the clock, naming the rule, and
# passes the one that waits on a signal or on a clock the test controls.
# And the rules that the app takes no input it is not given, issue #369.
# The fixtures are in scripts/testdata/lint, where go, the walks and the
# real lint runs do not look.
#
#   sh scripts/lint-test.sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fix=$root/scripts/testdata/lint
version=$(sed -n 's/^GOLANGCI_LINT_VERSION := //p' "$root/Makefile")
golangci=$root/.build/golangci-lint-$version/golangci-lint
eslint=$root/frontend/node_modules/.bin/eslint
fail=0

# expect WANT FOUND NAME COMMAND: COMMAND exits WANT, 0 or 1, and says
# FOUND. Any other exit is a linter that did not run, never a pass.
expect() {
	if out=$(eval "$4" 2>&1); then got=0; else got=$?; fi
	if [ "$got" = "$1" ] && printf '%s\n' "$out" | grep -qF -- "$2"; then
		printf 'ok  \t%s\n' "$3"
	else
		printf 'FAIL\t%s: exit %s, wanted %s and "%s"\n%s\n' "$3" "$got" "$1" "$2" "$out"
		fail=1
	fi
}

go="cd '$fix/go' && '$golangci' run --config '$root/.golangci.yml'"
expect 1 "forbidigo: 10" "a Go test that waits on the clock, on a deadline or on how long it took fails" "$go ./waits/..."
# The checks from outside the app build on a Mac with the outside tag
# only, so make lint reads them the way they build, as this does.
expect 1 "forbidigo: 1" "a Go test behind the outside tag that waits on the clock fails" "cd '$fix/go' && GOOS=darwin '$golangci' run --config '$root/.golangci.yml' --build-tags outside ./outside/..."
expect 0 "0 issues" "a Go test on signals or a synctest bubble passes" "$go ./signals/..."
expect 1 "forbidigo: 3" "Go code that hears other apps' keys or reads the clipboard through Wails fails, however it reaches it" "$go ./inputs/..."

# Read as if they were a walk and a test of the interface, so the config's
# own paths decide which rules hold.
walk="cd '$root/frontend' && '$eslint' --stdin --stdin-filename preview/walks/fixture.mjs <"
test="cd '$root/frontend' && '$eslint' --stdin --stdin-filename src/fixture.test.ts <"
expect 1 "playwright/no-wait-for-timeout" "a walk that waits on the clock fails" "$walk '$fix/walks/waits.mjs'"
expect 1 "4 problems" "a walk that hands a wait a timeout or a polling fails" "$walk '$fix/walks/deadlines.mjs'"
expect 0 "" "a walk on the page or page.clock passes" "$walk '$fix/walks/signals.mjs'"
expect 1 "no-restricted-globals" "an interface test that waits on the clock fails" "$test '$fix/walks/waits.test.ts'"
expect 1 "6 problems" "an interface test that polls with vi.waitFor or vi.waitUntil fails too" "$test '$fix/walks/waits.test.ts'"
expect 0 "" "an interface test on vi.useFakeTimers passes" "$test '$fix/walks/signals.test.ts'"
code="cd '$root/frontend' && '$eslint' --stdin --stdin-filename src/fixture.ts <"
expect 1 "4 problems" "the interface's code that opens the microphone, the camera or the screen, or reads the clipboard, fails" "$code '$fix/walks/inputs.ts'"

exit $fail
