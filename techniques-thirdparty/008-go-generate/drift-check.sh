#!/bin/sh
# Technique 8, habit three: fail CI when generated files drift.
# git status also catches new, untracked output, which
# git diff --exit-code would miss.
set -eu

go generate ./...
test -z "$(git status --porcelain)" ||
    { git status --short; exit 1; }
