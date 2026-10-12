#!/bin/sh
# Technique 1: CI fails when the Dockerfile's builder tag drifts
# from go.mod's toolchain line. Run it from the module root.
# The official golang image sets GOTOOLCHAIN=local, so this tag
# is the compiler that really builds the release.
set -u

# CI: the builder tag must match the toolchain line
want=$(go mod edit -json | jq -r .Toolchain)
grep -q " golang:${want#go} AS " Dockerfile ||
    { echo "builder is not $want" >&2; exit 1; }
