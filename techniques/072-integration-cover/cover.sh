#!/usr/bin/env bash
# Technique 72's script, with the book's names swapped for this
# directory's (bin/svc, ./cmd/svc). Run from this directory.
set -e
mkdir -p cov/int cov/unit cov/all
go build -cover -coverpkg=./... \
    -o bin/svc ./cmd/svc
GOCOVERDIR=cov/int ./bin/svc &
pid=$!
./scripts/smoke.sh
# main must catch SIGTERM and return (Technique 45)
kill -TERM "$pid"; wait "$pid"
ls cov/int/covcounters.* > /dev/null
go test -cover -coverpkg=./... ./... \
    -args -test.gocoverdir="$PWD/cov/unit"
go tool covdata merge -i=cov/int,cov/unit \
    -o cov/all
go tool covdata percent -i=cov/all
