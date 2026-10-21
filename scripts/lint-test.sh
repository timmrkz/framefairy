#!/bin/sh
# Checks that the linters refuse a test that waits on the clock, and pass
# one that waits on a signal, because a mistake in their settings is
# silent: CI goes green and the rule is words again.
#
#   sh scripts/lint-test.sh
#
# It runs golangci-lint with .golangci.yml, and ESLint with
# frontend/eslint.config.js and the lint script of frontend/package.json,
# on small projects of their own in a folder that is thrown away after,
# so nothing in the repository is touched. Each project keeps one known
# use the way the real lists do, in the config and in
# eslint-suppressions.json, to show a known use passes and a second one in
# the same file does not.
#
# It waits on nothing but the linters it runs, one after the other.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(dirname -- "$here")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

bin=$(sh "$here/golangci-lint.sh")
${MAKE:-make} -s --no-print-directory -C "$root" frontend/node_modules/.package-lock.json

# expect WANT NAME DIR COMMAND [FOUND...]: COMMAND, run in DIR, passes
# (WANT 0) or fails on what it found (WANT 1). A failure has to be the
# linter finding something, exit status 1, and has to name every FOUND.
# Any other status is the linter itself broken, which is never a pass.
expect() {
	want=$1 name=$2 dir=$3 cmd=$4
	shift 4
	if (cd "$dir" && eval "$cmd") >"$work/out" 2>&1; then got=0; else got=$?; fi
	ok=1
	[ "$got" = "$want" ] || ok=0
	for found in "$@"; do
		grep -qF -- "$found" "$work/out" || ok=0
	done
	if [ "$ok" = 1 ]; then
		printf 'ok  \t%s\n' "$name"
	else
		printf 'FAIL\t%s: wanted exit %s naming [%s], got exit %s:\n' "$name" "$want" "$*" "$got"
		sed 's/^/\t/' "$work/out" | grep -v 'Skipped 0 issues by rules' | head -40
		fail=1
	fi
}

# Go: a module of its own, with the repository's settings and one known
# use added to their list.
gomod=$work/go
mkdir -p "$gomod"
printf 'module fixture\n\ngo %s\n' "$(sed -n 's/^go //p' "$root/go.mod")" >"$gomod/go.mod"
awk '
	/^issues:/ {
		print "      - path: '\''^known_test\\.go$'\''"
		print "        source: '\''^\\s*time\\.Sleep\\(time\\.Millisecond\\)$'\''"
		print "        linters: [forbidigo]"
		print ""
	}
	{ print }
' "$root/.golangci.yml" >"$gomod/.golangci.yml"
golint="\"$bin\" run ./..."

# Code that is not a test may keep time, and a test may wait on a channel.
cat >"$gomod/fixture.go" <<'GO'
package fixture

import "time"

func Wait() { time.Sleep(time.Millisecond) }
GO
cat >"$gomod/known_test.go" <<'GO'
package fixture

import (
	"testing"
	"time"
)

func TestKnown(t *testing.T) {
	time.Sleep(time.Millisecond)
}
GO
cat >"$gomod/signal_test.go" <<'GO'
package fixture

import "testing"

func TestSignal(t *testing.T) {
	done := make(chan struct{})
	go func() { Wait(); close(done) }()
	<-done
}
GO
expect 0 "Go: a test that waits on a signal, code that sleeps, a known use" "$gomod" "$golint"

# Every way of reaching the clock, each named.
cat >"$gomod/clock_test.go" <<'GO'
package fixture

import (
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	time.Sleep(time.Millisecond)
	<-time.After(time.Millisecond)
	_ = time.Tick(time.Millisecond)
	time.NewTimer(time.Millisecond).Stop()
	time.NewTicker(time.Millisecond).Stop()
	time.AfterFunc(time.Millisecond, func() {}).Stop()
}
GO
expect 1 "Go: a test that sleeps, waits for After, Tick, a timer, a ticker or AfterFunc" "$gomod" "$golint" \
	'`time.Sleep`' '`time.After`' '`time.Tick`' '`time.NewTimer`' '`time.NewTicker`' '`time.AfterFunc`'
rm "$gomod/clock_test.go"

# Time under another name is still time.
cat >"$gomod/renamed_test.go" <<'GO'
package fixture

import (
	"testing"
	clock "time"
)

func TestRenamed(t *testing.T) {
	clock.Sleep(clock.Millisecond)
}
GO
expect 1 "Go: time.Sleep imported under another name" "$gomod" "$golint" "renamed_test.go:9"
rm "$gomod/renamed_test.go"

# The known use passes, a new one beside it does not.
cat >>"$gomod/known_test.go" <<'GO'

func TestNew(t *testing.T) {
	time.Sleep(2 * time.Millisecond)
}
GO
expect 1 "Go: a new sleep in a file that has a known one" "$gomod" "$golint" "known_test.go:13"

# The walks and the interface's tests: a frontend of their own, with the
# repository's settings, its lint script and its packages, and one known
# use in eslint-suppressions.json.
js=$work/js
mkdir -p "$js/preview/walks" "$js/src/lib"
cp "$root/frontend/eslint.config.js" "$root/frontend/package.json" "$js/"
ln -s "$root/frontend/node_modules" "$js/node_modules"
jslint="npm run --silent lint"
cat >"$js/eslint-suppressions.json" <<'JSON'
{
  "preview/walks/known.mjs": {
    "playwright/no-wait-for-timeout": {
      "count": 1
    }
  }
}
JSON
cat >"$js/preview/walks/known.mjs" <<'JS'
export async function known(page) {
  await page.waitForTimeout(100);
}
JS
# A clock the walk or the test controls is no clock.
cat >"$js/preview/walks/signal.mjs" <<'JS'
export async function signal(page) {
  await page.clock.install();
  await page.clock.runFor(1000);
  await page.waitForFunction(() => document.body.dataset.ready === "yes");
  await page.getByRole("button", { name: "Search" }).click();
}
JS
cat >"$js/src/lib/signal.test.ts" <<'TS'
import { expect, test, vi } from "vitest";

test("a fake clock", () => {
  vi.useFakeTimers();
  let fired: boolean = false;
  queueMicrotask(() => (fired = true));
  vi.advanceTimersByTime(100);
  expect(fired as boolean).toBe(true);
  vi.useRealTimers();
});
TS
expect 0 "walks: page.clock and waiting on the page, a known waitForTimeout, a test on vi's clock" "$js" "$jslint"

cat >"$js/preview/walks/clock.mjs" <<'JS'
export async function clock(page, main) {
  await page.waitForTimeout(100);
  await main.waitForTimeout(100);
  await new Promise((done) => setTimeout(done, 100));
  const t = Date.now() - performance.now();
  return t;
}
JS
expect 1 "walks: waitForTimeout on a page and on a second window, setTimeout, Date.now, performance.now" "$js" "$jslint" \
	"clock.mjs" "2:9  " "playwright/no-wait-for-timeout" "3:9  " "4:31 " "5:13 " "5:26 "
rm "$js/preview/walks/clock.mjs"

cat >"$js/src/lib/clock.test.ts" <<'TS'
import { test } from "vitest";

test("the clock", async () => {
  const from: number = performance.now();
  await new Promise<void>((done) => setInterval(done, 10));
  await new Promise<void>((done) => window.setTimeout(done, 10));
  return from;
});
TS
expect 1 "interface tests: performance.now, setInterval, window.setTimeout" "$js" "$jslint" \
	"clock.test.ts" "4:24" "5:37" "6:37"
rm "$js/src/lib/clock.test.ts"

cat >"$js/preview/walks/timers.mjs" <<'JS'
import { setTimeout as sleep } from "node:timers/promises";
export const wait = () => sleep(100);
JS
expect 1 "walks: Node's timers by import" "$js" "$jslint" "no-restricted-imports"
rm "$js/preview/walks/timers.mjs"

cat >>"$js/preview/walks/known.mjs" <<'JS'
export async function more(page) {
  await page.waitForTimeout(100);
}
JS
expect 1 "walks: a new waitForTimeout in a file that has a known one" "$js" "$jslint" "known.mjs"

if [ "$fail" = 0 ]; then
	echo "ok  	the linters refuse a test that waits on the clock"
fi
exit $fail
