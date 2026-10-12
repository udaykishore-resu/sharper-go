#!/usr/bin/env bash
# Drives the real binary over HTTP. Retries until it is listening.
set -euo pipefail
base="http://${SVC_ADDR:-127.0.0.1:8072}"
curl -fsS --retry 30 --retry-delay 0 --retry-connrefused \
    --retry-max-time 15 "$base/healthz" > /dev/null
curl -fsS "$base/v1/accounts/acc_1" > /dev/null
code=$(curl -s -o /dev/null -w '%{http_code}' "$base/v1/accounts/nope")
[ "$code" = 404 ]
