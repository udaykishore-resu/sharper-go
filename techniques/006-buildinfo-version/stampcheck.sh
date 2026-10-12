#!/bin/sh
# Technique 6: the release job's stamp check. It parses the build
# info as JSON (go version -m -json, Go 1.25+) and fails unless the
# binary names a VCS revision and was built from a clean tree.
# jq -e exits 1 and prints false otherwise.
# Run it from the module root after the release build.
set -eu

go version -m -json bin/ledgerd | jq -e '
    [.Settings[] | {(.Key): .Value}] | add
    | .["vcs.revision"] and .["vcs.modified"] == "false"'
