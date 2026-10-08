#!/bin/sh
set -eu
export GOTOOLCHAIN=local
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
    printf 'Run gofmt on:\n%s\n' "$unformatted"
    exit 1
fi
go vet ./...
go test -race ./...
