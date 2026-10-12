#!/bin/sh
# Technique 5: the build ledgerd ships with, kept in one script.
# Run from the module root. Same commit, toolchain, target,
# flags and environment give the same bytes.
set -eu

CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o bin/ledgerd ./cmd/ledgerd

# what does this binary carry?
go version -m bin/ledgerd
