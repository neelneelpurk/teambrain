#!/usr/bin/env bash
#
# check-coverage.sh enforces a per-package statement-coverage floor across the
# packages under internal/ that have their own tests. The PRD's Definition of
# Done requires >=80% on internal/*; this is the gate that proves it.
#
# Only "ok <pkg> ... coverage: <pct>%" result lines are gated. Test-only helper
# packages with no tests of their own (e.g. internal/testutil) are reported by
# `go test` without an "ok" prefix and are intentionally not gated. Any actual
# test failure is a hard failure.
#
# Usage: scripts/check-coverage.sh [threshold]   (threshold defaults to 80)
set -euo pipefail

threshold="${1:-80}"
# Capture output without aborting on a non-zero exit: real test failures are
# detected from the FAIL lines below, so we don't want unrelated toolchain noise
# (e.g. a missing covdata tool while measuring a no-test helper package) to mask
# the gate.
out="$(go test -cover ./internal/... 2>&1)" || true

# A failing test anywhere is a hard failure.
if grep -qE '^(FAIL|--- FAIL)' <<<"$out"; then
	grep -E '^(FAIL|--- FAIL)' <<<"$out" || true
	echo "coverage gate failed: tests did not pass"
	exit 1
fi

fail=0
found=0
while IFS= read -r line; do
	# Gate only result lines for packages that actually ran tests.
	[[ "$line" == ok*coverage:* ]] || continue
	pkg="$(awk '{print $2}' <<<"$line")"
	cov="$(sed -E 's/.*coverage: ([0-9.]+)%.*/\1/' <<<"$line")"
	[[ "$cov" =~ ^[0-9.]+$ ]] || continue
	found=1
	if awk -v c="$cov" -v t="$threshold" 'BEGIN { exit (c + 0 < t + 0) ? 1 : 0 }'; then
		printf 'ok    %-70s %s%%\n' "$pkg" "$cov"
	else
		printf 'FAIL  %-70s %s%% < %s%%\n' "$pkg" "$cov" "$threshold"
		fail=1
	fi
done <<<"$out"

if [ "$found" -eq 0 ]; then
	echo "no internal packages with coverage found (nothing to check yet)"
	exit 0
fi
if [ "$fail" -ne 0 ]; then
	echo "coverage gate failed: every tested internal/* package must be >= ${threshold}%"
	exit 1
fi
echo "coverage gate passed: all tested internal/* packages >= ${threshold}%"
