#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

export GOCACHE="${GOCACHE:-/tmp/vargasparse-gocache}"
mkdir -p "$GOCACHE"

go test -cover ./... | tee "$tmp"

check_threshold() {
	local pkg="$1"
	local min="$2"
	local line
	local pct

	line="$(grep -E "^ok[[:space:]]+${pkg}[[:space:]].*coverage: " "$tmp" | tail -n 1 || true)"
	if [[ -z "$line" ]]; then
		echo "❌ Coverage gate: missing package line for ${pkg}"
		return 1
	fi

	pct="$(echo "$line" | sed -E 's/.*coverage: ([0-9.]+)%.*/\1/')"
	if ! awk -v got="$pct" -v need="$min" 'BEGIN { exit !(got+0 >= need+0) }'; then
		echo "❌ Coverage gate: ${pkg} is ${pct}% (need >= ${min}%)"
		return 1
	fi
	echo "✅ ${pkg} coverage ${pct}% (>= ${min}%)"
}

check_threshold "vargasparse/internal/pipeline" 70
check_threshold "vargasparse/internal/deps" 80
check_threshold "vargasparse/internal/progress" 60
check_threshold "vargasparse/internal/renderer" 60
check_threshold "vargasparse/internal/llamacpp" 55
check_threshold "vargasparse/internal/ocr" 55
check_threshold "vargasparse/cmd/vargasparse" 40

echo "All risk-based coverage gates passed."
