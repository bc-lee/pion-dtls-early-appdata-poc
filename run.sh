#!/usr/bin/env bash
# Targets: release (v3.1.15), revision (v4@837949d), or local checkout
# (path:/path/to/dtls). Revisions like v4@837949d^ require DTLS_SRC.
set -u
root=$(cd "$(dirname "$0")" && pwd)

COUNT=${COUNT:-3}
DTLS_SRC=${DTLS_SRC:-}

targets=("$@")
if [ ${#targets[@]} -eq 0 ]; then
	targets=(v3.1.12 v3.1.13 v3.1.15 v4.0.0-rc.7)
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

i=0
for t in "${targets[@]}"; do
	i=$((i + 1))
	label=$t
	if [[ $t == path:* ]]; then
		src=${t#path:}
		if [ ! -f "$src/go.mod" ]; then
			echo "== $t: set a valid path to a Pion DTLS checkout" >&2
			continue
		fi
		mod=$(awk '$1 == "module" { print $2 }' "$src/go.mod")
		major=${mod##*/v}
		label="$t ($(git -C "$src" log -1 --format='%h %ad' --date=short 2>/dev/null))"
	elif [[ $t =~ ^v([0-9]+)@(.+)$ ]]; then
		major=${BASH_REMATCH[1]}
		mod=github.com/pion/dtls/v$major
		query=${BASH_REMATCH[2]}
		if [[ $query == *'^'* || $query == *'~'* ]]; then
			if [ -z "$DTLS_SRC" ] || ! full=$(git -C "$DTLS_SRC" rev-parse --verify --quiet "$query^{commit}" 2>/dev/null); then
				echo "== $t: set DTLS_SRC to a checkout that can resolve this revision" >&2
				continue
			fi
			query=$full
		fi
	elif [[ $t =~ ^v([0-9]+)\. ]]; then
		major=${BASH_REMATCH[1]}
		mod=github.com/pion/dtls/v$major
		query=$t
	else
		echo "== $t: unrecognized target"
		continue
	fi
	dir=$root/v$major
	if [ ! -d "$dir" ]; then
		echo "== $label: no $dir"
		continue
	fi

	work=$tmp/$i
	mkdir -p "$work"
	cp "$dir/go.mod" "$work/go.mod"
	[ -f "$dir/go.sum" ] && cp "$dir/go.sum" "$work/go.sum"

	if [[ $t == path:* ]]; then
		go mod edit -replace="$mod=$src" "$work/go.mod"
		setup=(go mod tidy -modfile="$work/go.mod")
	else
		setup=(go get -modfile="$work/go.mod" "$mod@$query")
	fi
	if ! (cd "$dir" && "${setup[@]}") >"$work/setup.log" 2>&1; then
		echo "== $label: ${setup[*]} failed"
		cat "$work/setup.log"
		continue
	fi
	if [[ $t != path:* ]]; then
		resolved=$(cd "$dir" && go list -m -modfile="$work/go.mod" "$mod" | awk '{ print $2 }')
		[ "$resolved" = "$t" ] || label="$t -> $resolved"
	fi

	echo "== $label (count=$COUNT)"
	out=$(cd "$dir" && go test -modfile="$work/go.mod" -count="$COUNT" -run TestEarlyApplicationData -v ./... 2>&1)
	grep -E '^(--- |    early_appdata_test|ok|FAIL|PASS)' <<<"$out"
	grep -q '^--- ' <<<"$out" || echo "$out"
done
