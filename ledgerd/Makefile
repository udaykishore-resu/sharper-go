.RECIPEPREFIX = >
VERSION ?= $(shell git describe --tags --always \
    --dirty 2>/dev/null || echo dev)
PKG     := example.com/ledgerd/internal/obs
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION)

.PHONY: all gen build test itest lint vuln check image
all: check build

gen:
> go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

build:
> CGO_ENABLED=0 go build -trimpath \
>   -ldflags '$(LDFLAGS)' -o bin/ledgerd ./cmd/ledgerd

test:
> go test -race -shuffle=on -count=1 ./...

itest:
> go test -race -count=1 -tags integration ./...

lint:
> golangci-lint run ./...

vuln:
> go tool govulncheck ./...

check: lint test vuln

image:
> docker buildx build --platform linux/amd64,linux/arm64 \
>   --secret id=netrc,src=$(HOME)/.netrc \
>   --build-arg VERSION=$(VERSION) -t ledgerd:$(VERSION) .
