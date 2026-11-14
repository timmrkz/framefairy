#!/bin/sh
# Checks the rule in build-ended.sh against the answers GitHub gives, by a
# gh of its own on the PATH, so nothing is asked of GitHub. A mistake in
# the rule is silent: the Updates page says a build is coming that never
# will, or never says one is.
#
#   sh scripts/build-ended-test.sh
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

# The gh here answers what the test says GitHub knows: STATUS, the state
# of Ready to update to, and RUN, the newest run's status and conclusion.
# FAILS names a call that fails, the way one does when GitHub is down.
mkdir "$work/bin"
cat >"$work/bin/gh" <<'GH'
#!/bin/sh
case "$*" in
*/status\ *)
	[ "${FAILS-}" != status ] || exit 1
	echo "${STATUS-}"
	;;
*/runs\?*)
	[ "${FAILS-}" != runs ] || exit 1
	echo "${RUN-}"
	;;
*)
	echo "unexpected gh $*" >&2
	exit 1
	;;
esac
GH
chmod +x "$work/bin/gh"

# expect WORD WHY, then what GitHub knows as VAR=value.
expect() {
	want=$1
	why=$2
	shift 2
	got=$(env PATH="$work/bin:$PATH" GH_REPO=o/r "$@" sh "$here/build-ended.sh" 0123456789ab 2>/dev/null)
	if [ "$got" != "$want" ]; then
		echo "build-ended.sh: $why: said $got, want $want" >&2
		fail=1
	fi
}

expect ended "a build that failed" STATUS= RUN="completed failure"
expect ended "a build cancelled by hand" STATUS= RUN="completed cancelled"
expect ended "a build that timed out" STATUS= RUN="completed timed_out"
expect ended "a build the signer refused" STATUS=failure RUN="completed success"
expect coming "a build still running" STATUS= RUN="in_progress null"
expect coming "a build waiting for a runner" STATUS= RUN="queued null"
expect coming "a build that ended well and is being published" STATUS= RUN="completed success"
expect coming "a pull request's build already published" STATUS=success RUN="completed success"
expect coming "a commit with no run yet" STATUS= RUN="null null"
expect coming "statuses that cannot be read" FAILS=status RUN="completed failure"
expect coming "runs that cannot be read" FAILS=runs STATUS=

[ "$fail" = 0 ] || exit 1
printf 'ok  \tthe rule for a build that ended\n'
