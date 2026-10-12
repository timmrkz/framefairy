#!/bin/sh
# Checks scripts/go-shard.sh: split two ways and three ways, the shares
# hold every test of the packages once, and none twice. A share that lost
# tests would pass in CI having run less, which nobody would see.
#
#   sh scripts/go-shard-test.sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# A go that lists for real and, asked to run, says what it was asked to run.
cat > "$work/go" <<'GO'
#!/bin/sh
case " $* " in *" -list "*) exec go "$@" ;; esac
while [ $# -gt 0 ]; do
	if [ "$1" = -run ]; then echo "$2"; exit 0; fi
	shift
done
GO
chmod +x "$work/go"

packages="./licence ./updates ./licence/signer ./train"
go test -list '.*' $packages | grep -E '^(Test|Fuzz|Example)' | sort > "$work/all"
failed=0
for n in 2 3; do
	: > "$work/got"
	i=1
	while [ "$i" -le "$n" ]; do
		run=$(GO="$work/go" GOSHARD="$i/$n" sh scripts/go-shard.sh $packages | tail -n 1)
		grep -E "$run" "$work/all" >> "$work/got" || true
		i=$((i + 1))
	done
	sort "$work/got" > "$work/got.sorted"
	if ! cmp -s "$work/all" "$work/got.sorted"; then
		echo "FAIL	$n shares do not hold every test once:"
		diff "$work/all" "$work/got.sorted" || true
		failed=1
	else
		printf 'ok  \t%s shares hold all %s tests once\n' "$n" "$(wc -l < "$work/all" | tr -d ' ')"
	fi
done
exit "$failed"
