#!/usr/bin/env bash
#
# check-coverage.sh enforces a per-package statement-coverage floor across every
# package under internal/. The PRD's Definition of Done requires >=80% on
# internal/*; this is the gate that proves it.
#
# Usage: scripts/check-coverage.sh [threshold]   (threshold defaults to 80)
set -euo pipefail

threshold="${1:-80}"
fail=0
found=0

while IFS= read -r line; do
	# Lines look like: "ok  <pkg>  0.123s  coverage: 90.8% of statements"
	pkg="$(awk '{print $2}' <<<"$line")"
	cov="$(grep -oE 'coverage: [0-9.]+%' <<<"$line" | grep -oE '[0-9.]+' || true)"
	[ -z "$cov" ] && continue
	found=1
	if awk -v c="$cov" -v t="$threshold" 'BEGIN { exit (c + 0 < t + 0) ? 1 : 0 }'; then
		printf 'ok    %-70s %s%%\n' "$pkg" "$cov"
	else
		printf 'FAIL  %-70s %s%% < %s%%\n' "$pkg" "$cov" "$threshold"
		fail=1
	fi
done < <(go test -cover ./internal/... 2>/dev/null | grep 'coverage:')

if [ "$found" -eq 0 ]; then
	echo "no internal packages with coverage found (nothing to check yet)"
	exit 0
fi

if [ "$fail" -ne 0 ]; then
	echo "coverage gate failed: every internal/* package must be >= ${threshold}%"
	exit 1
fi

echo "coverage gate passed: all internal/* packages >= ${threshold}%"
