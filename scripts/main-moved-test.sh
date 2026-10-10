#!/bin/sh
# Checks main-moved.sh on a repository of its own, because a mistake in it
# is silent: a pull request sits in conflict and the session watching it is
# never told.
#
#   sh scripts/main-moved-test.sh
#
# It stands in for gh with a script that lists the pull requests and keeps
# the comments in files, so nothing reaches GitHub.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
moved=$here/main-moved.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1

mkdir "$work/bin" "$work/comments"
cat >"$work/bin/gh" <<'GH'
#!/bin/sh
# pr list: the lines main-moved.sh asks jq for.
# pr view N: the comments on N. pr comment N --body B: adds one.
case "$1 $2" in
"pr list") cat "$GH_DIR/prs" ;;
"pr view") cat "$GH_DIR/comments/$3" 2>/dev/null || true ;;
"pr comment")
	printf '%s\n' "$5" >>"$GH_DIR/comments/$3"
	echo x >>"$GH_DIR/comments/$3.count"
	;;
*) echo "gh: $*" >&2 && exit 1 ;;
esac
GH
chmod +x "$work/bin/gh"
export GH_DIR="$work" PATH="$work/bin:$PATH"

git init -q --bare -b main "$work/origin.git"
git clone -q "$work/origin.git" "$work/repo" 2>/dev/null
cd "$work/repo"

commit() {
	printf '%s\n' "$2" >"$1"
	git add "$1"
	git commit -q -m "$3"
}

commit plan.md "row 1" "start"
commit code.go "package a" "code"
git push -q origin main

# clean: touches a file main does not.
git checkout -q -b clean main
commit other.md "other" "clean change"
git push -q origin clean
# clash: changes the row main changes next.
git checkout -q -b clash main
commit plan.md "row 2 from clash" "clash change"
git push -q origin clash
# late: started after main moved, from the main before.
git checkout -q -b late main
commit plan.md "row 2 from late" "late change"
git push -q origin late

git checkout -q main
commit plan.md "row 2 from main" "main moves"
git push -q origin main

# up: already has the newest main in it.
git checkout -q -b up main
commit more.md "more" "up change"
git push -q origin up
git checkout -q main

cat >"$work/prs" <<PRS
1 clean false
2 clash false
3 late false
4 up false
5 fork true
PRS

count() {
	if [ -f "$work/comments/$1.count" ]; then
		wc -l <"$work/comments/$1.count" | tr -d ' '
	else
		echo 0
	fi
}

expect() {
	name=$1
	pr=$2
	want=$3
	got=$(count "$pr")
	if [ "$got" = "$want" ]; then
		printf 'ok  \t%-46s #%s has %s\n' "$name" "$pr" "$got"
	else
		printf 'FAIL\t%-46s #%s has %s, want %s\n' "$name" "$pr" "$got" "$want"
		fail=1
	fi
}

sh "$moved" 3 >/dev/null
expect "one pull request: the one asked about" 3 1
expect "one pull request: no other" 2 0

sh "$moved" >/dev/null
expect "every pull request: in conflict" 2 1
expect "every pull request: told once about one pair" 3 1
expect "every pull request: merges cleanly" 1 0
expect "every pull request: has main" 4 0
expect "every pull request: a fork is not ours" 5 0

if grep -qF '`plan.md`' "$work/comments/2"; then
	printf 'ok  \t%-46s\n' "names the file in conflict"
else
	printf 'FAIL\t%-46s\n' "names the file in conflict"
	fail=1
fi

sh "$moved" >/dev/null
expect "run twice for the same push" 2 1

# The branch takes main in, and then main moves into it again.
git checkout -q clash
git merge -q -X theirs main -m "take main" >/dev/null
git push -q origin clash
sh "$moved" 2 >/dev/null
expect "main taken in" 2 1
commit code.go "package b" "clash again"
git checkout -q main
commit code.go "package c" "main changes code"
git push -q origin main clash
sh "$moved" 2 >/dev/null
expect "in conflict again after main moves" 2 2

if sh "$moved" 1x >/dev/null 2>&1; then
	printf 'FAIL\t%-46s\n' "refuses what is not a number"
	fail=1
else
	printf 'ok  \t%-46s\n' "refuses what is not a number"
fi

exit $fail
