# sharper-go

Companion code for *Sharper Go: 101 Modern Techniques for Production Services*: every listing made runnable, plus the complete production-grade `ledgerd` service.

By Udaykishore Resu. Each technique in the book has a small, tested example here, and the techniques you would combine in a real service are wired together in `ledgerd`, a double-entry ledger API with Postgres, idempotent transfers, an outbox, structured logs, traces, metrics and Kubernetes manifests.

## Requirements

- **Go 1.26 or later.** Every module's `go.mod` says `go 1.26.0` and `toolchain go1.27.2`; with the default `GOTOOLCHAIN=auto`, an older Go fetches 1.27.2 for you (Technique 1).
- **Tested with Go 1.27.2.** A few examples use Go 1.27 APIs (`encoding/json/v2`, `httptest.NewTestServer`, `synctest.Sleep`); those files carry a `//go:build go1.27` line.
- **Version tags are tested.** `go test ./internal/versiontags` (in `techniques/`) vets each standard-library technique with its `go` line set to the Go version on the book's card.
- **Docker** (optional) for `docker compose` and testcontainers-based integration tests. A plain Postgres works too (see below).
- A C compiler for `go test -race` and for Technique 100's cgo path. Both fall back cleanly with `CGO_ENABLED=0`.

## Layout

```text
sharper-go/
├── ledgerd/                 the capstone service (Technique 101, Appendix C)
│   ├── cmd/ledgerd/         main, startup order, servers and drain
│   ├── internal/ledger/     domain types, errors, validation, amounts
│   ├── internal/config/     boot config from env, live config reload
│   ├── internal/store/      pgx v5, InTx/RunTx, idempotency, outbox, migrations
│   ├── internal/httpapi/    routes, middleware, idempotency, probes
│   ├── internal/obs/        slog + trace IDs, Prometheus, OpenTelemetry, pprof/expvar
│   ├── internal/sink/       outbox event sinks (registry + log sink)
│   ├── k8s/                 Deployment, Service, migration Job
│   ├── Dockerfile, Makefile, compose.yaml, sqlc.yaml, .golangci.yml
├── techniques/              standard-library-only examples (one module)
└── techniques-thirdparty/   examples that need pgx, OTel, Prometheus, gRPC,
                             client-go, controller-runtime, cobra, testcontainers,
                             x/sync, x/time (one module)
```

Each `techniques*/NNN-short-name/` directory holds the book's code for Technique NNN, wrapped just enough to compile, and a test (or benchmark) that demonstrates the claim the technique makes. Fragments keep the book's names, so you can read the page and the file side by side.

## Technique map

The book's source calls them "Hack N"; the printed book says "Technique N". Same numbers.

| # | Technique | Code |
|---|-----------|------|
| 1 | Pin the compiler per repo with the toolchain line | [techniques/001-toolchain-line](techniques/001-toolchain-line); [ledgerd/go.mod](ledgerd/go.mod) (`toolchain` line); [Dockerfile](ledgerd/Dockerfile) |
| 2 | Track dev tools with the tool directive | [ledgerd/go.mod](ledgerd/go.mod) (`tool` block); [techniques-thirdparty/go.mod](techniques-thirdparty/go.mod) |
| 3 | go work for multi-module local development | [techniques/003-go-work](techniques/003-go-work) |
| 4 | govulncheck in CI | [techniques/004-govulncheck](techniques/004-govulncheck); [ledgerd/Makefile](ledgerd/Makefile) (`vuln`) |
| 5 | Reproducible, version-stamped binaries | [techniques/005-reproducible-build](techniques/005-reproducible-build); [ledgerd/Makefile](ledgerd/Makefile) (`build`) |
| 6 | debug.ReadBuildInfo for a free version endpoint | [techniques/006-buildinfo-version](techniques/006-buildinfo-version); also [ledgerd/internal/obs/version.go](ledgerd/internal/obs/version.go) |
| 7 | Build tags for platforms and integration tests | [techniques/007-build-tags](techniques/007-build-tags) |
| 8 | go generate that's actually reproducible | [techniques-thirdparty/008-go-generate](techniques-thirdparty/008-go-generate) |
| 9 | GOPRIVATE, GOPROXY and GOFLAGS for corporate setups | [techniques/009-goprivate](techniques/009-goprivate); [ledgerd/Dockerfile](ledgerd/Dockerfile) (netrc secret) |
| 10 | A baseline vet + staticcheck + golangci-lint config | [ledgerd/.golangci.yml](ledgerd/.golangci.yml) |
| 11 | Range over integers | [techniques/011-range-over-int](techniques/011-range-over-int) |
| 12 | Per-iteration loop variables: delete every v := v | [techniques-thirdparty/012-loopvar](techniques-thirdparty/012-loopvar) |
| 13 | Range-over-func iterators and the iter package | [techniques/013-iterators](techniques/013-iterators) |
| 14 | slices and maps instead of hand-written loops | [techniques/014-slices-maps](techniques/014-slices-maps) |
| 15 | min, max and clear builtins | [techniques/015-min-max-clear](techniques/015-min-max-clear) |
| 16 | Generic constraints that stay readable | [techniques/016-generic-constraints](techniques/016-generic-constraints) |
| 17 | Generic type aliases | [techniques/017-generic-aliases](techniques/017-generic-aliases) |
| 18 | cmp.Or for defaults | [techniques/018-cmp-or](techniques/018-cmp-or); also [ledgerd/internal/config](ledgerd/internal/config) |
| 19 | Interning with the unique package | [techniques/019-unique](techniques/019-unique) |
| 20 | The omitzero JSON tag | [techniques/020-omitzero](techniques/020-omitzero); Part II synthesis export: [techniques/synth-02-export](techniques/synth-02-export) |
| 21 | Wrap with %w, inspect with errors.Is and errors.As | [techniques-thirdparty/021-wrap-errors](techniques-thirdparty/021-wrap-errors) |
| 22 | errors.Join for multi-errors | [techniques/022-errors-join](techniques/022-errors-join); also [ledgerd/internal/ledger](ledgerd/internal/ledger) |
| 23 | Sentinel vs typed errors: a one-line decision rule | [techniques/023-sentinel-vs-typed](techniques/023-sentinel-vs-typed) |
| 24 | Errors that carry HTTP and gRPC status | [techniques/024-error-status](techniques/024-error-status); also [ledgerd/internal/httpapi/errors.go](ledgerd/internal/httpapi/errors.go) |
| 25 | Functional options without over-engineering | [techniques/025-functional-options](techniques/025-functional-options) |
| 26 | Accept interfaces, return structs (and the exceptions) | [techniques/026-accept-interfaces](techniques/026-accept-interfaces) |
| 27 | Make the zero value useful | [techniques/027-zero-value](techniques/027-zero-value) |
| 28 | internal/ as an enforced API contract | [ledgerd/internal](ledgerd/internal) |
| 29 | // Deprecated: comments tools understand | [techniques/029-deprecated](techniques/029-deprecated) |
| 30 | errgroup with context and SetLimit | [techniques-thirdparty/030-errgroup](techniques-thirdparty/030-errgroup) |
| 31 | sync.WaitGroup.Go (1.25) | [techniques/031-waitgroup-go](techniques/031-waitgroup-go) |
| 32 | A bounded worker pool that stops on cancel | [techniques/032-worker-pool](techniques/032-worker-pool) |
| 33 | Pipelines with clean cancellation | [techniques-thirdparty/033-pipelines](techniques-thirdparty/033-pipelines) |
| 34 | context.WithoutCancel and context.AfterFunc (1.21) | [techniques/034-without-cancel](techniques/034-without-cancel) |
| 35 | context.WithCancelCause for better shutdown errors | [techniques/035-cancel-cause](techniques/035-cancel-cause) |
| 36 | sync.OnceValue and sync.OnceFunc (1.21) | [techniques/036-oncevalues](techniques/036-oncevalues) |
| 37 | singleflight to collapse duplicate calls | [techniques-thirdparty/037-singleflight](techniques-thirdparty/037-singleflight) |
| 38 | Rate limiting with x/time/rate | [techniques-thirdparty/038-rate-limit](techniques-thirdparty/038-rate-limit) |
| 39 | atomic.Pointer for hot-swappable config | [techniques/039-atomic-pointer](techniques/039-atomic-pointer) |
| 40 | Catch goroutine leaks in tests | [techniques/040-goroutine-leaks](techniques/040-goroutine-leaks) |
| 41 | Channel or mutex: a decision table | [techniques/041-channel-or-mutex](techniques/041-channel-or-mutex) |
| 42 | The race detector in CI, the cheap way | [techniques/042-race-ci](techniques/042-race-ci); also [ledgerd/Makefile](ledgerd/Makefile) (`test`) |
| 43 | Method and wildcard routing in net/http ServeMux | [techniques/043-servemux-routing](techniques/043-servemux-routing); also [ledgerd/internal/httpapi/api.go](ledgerd/internal/httpapi/api.go) |
| 44 | The four http.Server timeouts you must set | [techniques/044-server-timeouts](techniques/044-server-timeouts); also [ledgerd/internal/httpapi/server.go](ledgerd/internal/httpapi/server.go) |
| 45 | Graceful shutdown with signal.NotifyContext | [techniques/045-graceful-shutdown](techniques/045-graceful-shutdown); also [ledgerd/cmd/ledgerd/serve.go](ledgerd/cmd/ledgerd/serve.go) |
| 46 | Reuse one http.Client and tune its Transport | [techniques/046-http-client](techniques/046-http-client) |
| 47 | Middleware chains without a framework | [techniques-thirdparty/047-middleware-chain](techniques-thirdparty/047-middleware-chain); also [ledgerd/internal/httpapi/middleware.go](ledgerd/internal/httpapi/middleware.go) |
| 48 | http.MaxBytesReader and request limits | [techniques/048-max-bytes](techniques/048-max-bytes); also [ledgerd/internal/httpapi/decode.go](ledgerd/internal/httpapi/decode.go) |
| 49 | Health, readiness and build-info endpoints | [techniques/049-health-endpoints](techniques/049-health-endpoints); also [ledgerd/internal/httpapi/health.go](ledgerd/internal/httpapi/health.go) |
| 50 | gRPC interceptors for auth, logging and metrics | [techniques-thirdparty/050-grpc-interceptors](techniques-thirdparty/050-grpc-interceptors) |
| 51 | Retries with backoff, jitter and a retry budget | [techniques/051-retries](techniques/051-retries) |
| 52 | Idempotency middleware for POST endpoints | [techniques/052-idempotency](techniques/052-idempotency); also [ledgerd/internal/httpapi/idempotency.go](ledgerd/internal/httpapi/idempotency.go), [store/idem.go](ledgerd/internal/store/idem.go) |
| 53 | os.Root for traversal-safe file access | [techniques/053-os-root](techniques/053-os-root) |
| 54 | database/sql pool settings that matter | [techniques/054-sql-pool](techniques/054-sql-pool); also [ledgerd/internal/store/store.go](ledgerd/internal/store/store.go) |
| 55 | pgx for Postgres | [techniques-thirdparty/055-pgx](techniques-thirdparty/055-pgx); also [ledgerd/internal/store/store.go](ledgerd/internal/store/store.go) |
| 56 | sqlc: type-safe Go from plain SQL | [ledgerd/sqlc.yaml](ledgerd/sqlc.yaml), [queries.sql](ledgerd/internal/store/queries.sql), [sqlcdb](ledgerd/internal/store/sqlcdb) |
| 57 | A transaction helper that can't leak | [techniques-thirdparty/057-tx-helper](techniques-thirdparty/057-tx-helper); also [ledgerd/internal/store/tx.go](ledgerd/internal/store/tx.go) |
| 58 | The outbox pattern in Go | [ledgerd/internal/store/relay.go](ledgerd/internal/store/relay.go), [migrations](ledgerd/internal/store/migrations) |
| 59 | Stream-decode huge JSON payloads | [techniques/059-stream-json](techniques/059-stream-json) |
| 60 | What encoding/json/v2 changes in Go 1.27 | [techniques/060-json-v2](techniques/060-json-v2) |
| 61 | Ship migrations inside the binary with embed.FS | [techniques-thirdparty/061-embed-migrations](techniques-thirdparty/061-embed-migrations); also [ledgerd/internal/store/migrate.go](ledgerd/internal/store/migrate.go) |
| 62 | Table tests with named subtests | [techniques/062-table-tests](techniques/062-table-tests); also [ledgerd/internal/ledger/ledger_test.go](ledgerd/internal/ledger/ledger_test.go) |
| 63 | t.Context (1.24) and t.Cleanup | [techniques/063-t-context](techniques/063-t-context) |
| 64 | testing/synctest for time-dependent code (1.25) | [techniques/064-synctest](techniques/064-synctest) |
| 65 | Golden files with an -update flag | [techniques/065-golden-files](techniques/065-golden-files) |
| 66 | httptest servers for client code | [techniques/066-httptest-server](techniques/066-httptest-server) |
| 67 | Native fuzzing | [techniques/067-native-fuzzing](techniques/067-native-fuzzing); also [ledgerd/internal/ledger](ledgerd/internal/ledger) (`FuzzParseAmount`) |
| 68 | testcontainers-go for real integration tests | [techniques-thirdparty/068-testcontainers](techniques-thirdparty/068-testcontainers); also [ledgerd/internal/store/store_integration_test.go](ledgerd/internal/store/store_integration_test.go) |
| 69 | testdata/ fixtures done right | [techniques/069-testdata](techniques/069-testdata) |
| 70 | Benchmarks with b.Loop (1.24) | [techniques/070-b-loop](techniques/070-b-loop) |
| 71 | benchstat: compare runs honestly | shell only; benchstat is a `tool` in [ledgerd/go.mod](ledgerd/go.mod); output shape checked in [ledgerd/internal/ledger/benchstat_review_test.go](ledgerd/internal/ledger/benchstat_review_test.go) |
| 72 | Coverage for integration tests with go build -cover | [techniques/072-integration-cover](techniques/072-integration-cover) (`cover.sh`, run by its test) |
| 73 | pprof in production, safely | [techniques/073-pprof-production](techniques/073-pprof-production) |
| 74 | Profile-guided optimization (PGO) | [techniques/074-pgo](techniques/074-pgo) (end-to-end with `PGO_E2E=1`; slow) |
| 75 | Read escape analysis with -gcflags=-m | [techniques/075-escape-analysis](techniques/075-escape-analysis) |
| 76 | Preallocate slices and maps | [techniques/076-preallocate](techniques/076-preallocate) |
| 77 | Reuse strings.Builder and bytes.Buffer | [techniques/077-buffer-reuse](techniques/077-buffer-reuse) |
| 78 | sync.Pool: when it helps, when it hurts | [techniques/078-sync-pool](techniques/078-sync-pool) |
| 79 | GOMEMLIMIT for containers | [techniques/079-gomemlimit](techniques/079-gomemlimit), [ledgerd/k8s/deployment.yaml](ledgerd/k8s/deployment.yaml) |
| 80 | Container-aware GOMAXPROCS (1.25) | [techniques/080-gomaxprocs](techniques/080-gomaxprocs) |
| 81 | Execution traces and the flight recorder (1.25) | [techniques/081-flight-recorder](techniques/081-flight-recorder) |
| 82 | Weak pointers and runtime.AddCleanup (1.24) | [techniques/082-weak-pointers](techniques/082-weak-pointers) |
| 83 | Struct field ordering to shrink memory | [techniques/083-struct-layout](techniques/083-struct-layout) |
| 84 | Keep reflection out of hot paths | [techniques/084-reflection-plans](techniques/084-reflection-plans) |
| 85 | Structured logging with log/slog | [techniques/085-slog](techniques/085-slog); also [ledgerd/internal/obs/log.go](ledgerd/internal/obs/log.go) |
| 86 | Custom slog handlers: redact secrets, inject trace IDs | [techniques-thirdparty/086-slog-handlers](techniques-thirdparty/086-slog-handlers); also [ledgerd/internal/obs/log.go](ledgerd/internal/obs/log.go) |
| 87 | OpenTelemetry tracing in 30 lines | [techniques-thirdparty/087-otel-tracing](techniques-thirdparty/087-otel-tracing); also [ledgerd/internal/obs/trace.go](ledgerd/internal/obs/trace.go) |
| 88 | The four Prometheus metrics every service needs | [techniques-thirdparty/088-prometheus-metrics](techniques-thirdparty/088-prometheus-metrics); also [ledgerd/internal/obs/metrics.go](ledgerd/internal/obs/metrics.go) |
| 89 | expvar for zero-dependency debugging | [techniques/089-expvar](techniques/089-expvar); also [ledgerd/internal/obs/debug.go](ledgerd/internal/obs/debug.go) |
| 90 | Panic-recovery middleware that logs the stack | [techniques/090-panic-recovery](techniques/090-panic-recovery); also [ledgerd/internal/httpapi/recover.go](ledgerd/internal/httpapi/recover.go) |
| 91 | Runtime config reload without restarts | [techniques/091-config-reload](techniques/091-config-reload); also [ledgerd/internal/config/live.go](ledgerd/internal/config/live.go) |
| 92 | Tiny static images: distroless and scratch | [ledgerd/Dockerfile](ledgerd/Dockerfile) |
| 93 | Cross-compile and build multi-arch images | [ledgerd/Dockerfile](ledgerd/Dockerfile), [Makefile](ledgerd/Makefile) (`image`) |
| 94 | client-go informers instead of polling the API server | [techniques-thirdparty/094-informers](techniques-thirdparty/094-informers) |
| 95 | controller-runtime reconciler patterns | [techniques-thirdparty/095-controller-runtime](techniques-thirdparty/095-controller-runtime) |
| 96 | Write a kubectl plugin | [techniques-thirdparty/096-kubectl-plugin](techniques-thirdparty/096-kubectl-plugin) |
| 97 | CLIs: cobra, or just flag | [techniques/097-cli-flag](techniques/097-cli-flag) |
| 98 | Embed static assets and templates | [techniques/098-embed-assets](techniques/098-embed-assets) |
| 99 | Plugins via interfaces and a registry, not the plugin package | [techniques/099-plugin-registry](techniques/099-plugin-registry); also [ledgerd/internal/sink](ledgerd/internal/sink) |
| 100 | cgo: when to reach for it and how to contain it | [techniques/100-cgo](techniques/100-cgo) |
| 101 | Wire it all together: a production-ready ledgerd skeleton | [ledgerd/](ledgerd) |

## Run ledgerd locally

With Docker:

```sh
cd ledgerd
docker compose up --build
```

Compose starts Postgres, runs `ledgerd migrate` once (as the Kubernetes Job does), seeds two demo accounts (`acc_1`, `acc_2`; the `house` account comes from a migration), and starts the API on `:8080` with metrics on `:9090`. Then:

```sh
TOKEN='Authorization: Bearer dev-token-0123456789'

# Fund acc_1 from the house account, then retry with the same key:
# the second call replays (Idempotent-Replayed: true), money moves once.
curl -i -X POST localhost:8080/v1/transfers -H "$TOKEN" \
  -H 'Idempotency-Key: k-1' -d '{"from":"house","to":"acc_1","amount":5000}'
curl -i -X POST localhost:8080/v1/transfers -H "$TOKEN" \
  -H 'Idempotency-Key: k-1' -d '{"from":"house","to":"acc_1","amount":5000}'

curl -s localhost:8080/v1/accounts/acc_1 -H "$TOKEN"
curl -s localhost:8080/v1/accounts/acc_1/entries -H "$TOKEN"
curl -s localhost:8080/readyz; curl -s localhost:8080/version
curl -s localhost:9090/metrics | grep ledgerd_http
```

Without Docker, point it at any Postgres:

```sh
cd ledgerd
make build
export DATABASE_URL='postgres://user:pass@localhost:5432/ledgerd?sslmode=disable'
export API_KEYS='demo:dev-token-0123456789'
./bin/ledgerd migrate
./bin/ledgerd            # serve; SIGTERM drains for 5s, then shuts down
```

Configuration is environment only: `DATABASE_URL` (required), `API_KEYS` (`client:token,...`, tokens of 16+ characters), `LOG_LEVEL`, `REQUEST_TIMEOUT` (10s), `SHUTDOWN_GRACE` (20s), `DB_MAX_CONNS` (12), `EVENT_SINK` (`log`), and `LEDGERD_LIVE_CONFIG`, an optional JSON file reloaded every 10s and on SIGHUP (`{"version":"v1","log_level":"INFO","max_transfer_cents":100000}`). Tracing exports over OTLP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. pprof and expvar listen on `127.0.0.1:6060` only.

## Tests

```sh
# ledgerd: unit tests, then integration tests against real Postgres
cd ledgerd
make test                                     # go test -race -shuffle=on -count=1 ./...
make itest                                    # -tags integration; starts postgres:17 via testcontainers
LEDGERD_TEST_DATABASE_URL='postgres://...' make itest   # or reuse a running server

# standard-library techniques
cd techniques && go vet ./... && go test -race ./...

# third-party techniques
cd techniques-thirdparty && go vet ./... && go test -race ./...
PGTEST_DATABASE_URL='postgres://...' go test -race -tags integration ./...
```

Integration tests sit behind the `integration` build tag (Technique 7). Without a URL they start a container with testcontainers-go (Technique 68), so they need Docker.

## Skills

Skills this repository demonstrates:

- **Go concurrency:** errgroup with limits, worker pools and pipelines that stop on cancel, `WaitGroup.Go`, context causes, `WithoutCancel`/`AfterFunc`, singleflight, token-bucket rate limiting, `atomic.Pointer`, goroutine-leak tests.
- **net/http services:** method and wildcard routing, the four server timeouts, graceful shutdown, middleware ordering, body limits, health/readiness probes, idempotency keys, retries with jitter and budgets, `os.Root`.
- **Postgres with pgx v5:** pool sizing, typed errors, a transaction helper that can't leak, serializable retries, the outbox pattern, sqlc, embedded goose migrations.
- **Observability:** `log/slog` with redaction and trace IDs, OpenTelemetry tracing, Prometheus metrics with bounded cardinality, expvar, panic recovery, pprof and the flight recorder.
- **Kubernetes and platform tooling:** client-go informers, controller-runtime reconcilers, a kubectl plugin with cobra and cli-runtime, distroless multi-arch images, probes, `GOMEMLIMIT`, migration Jobs.
- **Testing:** table tests, `t.Context`, `testing/synctest`, golden files, `httptest.NewTestServer`, native fuzzing, testcontainers, testdata fixtures, `b.Loop` benchmarks.
- **Performance and profiling:** escape analysis, preallocation, buffer reuse, `sync.Pool`, struct layout, weak pointers, reflection plan caches, PGO, container-aware `GOMAXPROCS`.
- **Toolchain hygiene:** toolchain pinning, the `tool` directive, reproducible stamped builds, build tags, reproducible `go generate`, govulncheck, golangci-lint v2.

GitHub topics: `go`, `golang`, `production`, `kubernetes`, `observability`, `book-companion`, `postgresql`, `pgx`, `opentelemetry`, `prometheus`, `slog`, `concurrency`, `testing`, `controller-runtime`

## Errata

Found a mistake in the book or the code? Open a GitHub issue on this repository, with the technique number and, for the book, the page.

## License

MIT. See [LICENSE](LICENSE).
