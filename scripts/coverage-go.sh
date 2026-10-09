#!/bin/sh
set -eu
export GOTOOLCHAIN=local
coverage_file=$(mktemp "${TMPDIR:-/tmp}/harness-ctl-coverage.XXXXXX")
trap 'rm -f "$coverage_file"' EXIT HUP INT TERM
go test -race -coverpkg=./... -coverprofile="$coverage_file" ./...
go tool cover -func="$coverage_file"
awk 'NR > 1 { statements[$1] = $2; counts[$1] += $3 }
END {
    for (block in statements) {
        total += statements[block]
        if (counts[block] > 0) covered += statements[block]
    }
    printf "Covered %d of %d statements. Target: 100%%.\n", covered, total
    if (covered != total || total == 0) exit 1
}' "$coverage_file"
