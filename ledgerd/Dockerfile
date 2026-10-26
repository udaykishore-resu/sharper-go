# Technique 101 / Appendix C.4: a static binary on distroless,
# built natively for each architecture (Techniques 92 and 93).
# The builder tag copies go.mod's toolchain line, because the
# official image sets GOTOOLCHAIN=local and never switches.
FROM --platform=$BUILDPLATFORM golang:1.27.3 AS build
ARG TARGETOS TARGETARCH VERSION=dev
ENV GOPRIVATE=git.corp.example.com/*
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=netrc,target=/root/.netrc \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w \
    -X example.com/ledgerd/internal/obs.Version=$VERSION" \
    -o /out/ledgerd ./cmd/ledgerd

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/ledgerd /ledgerd
ENTRYPOINT ["/ledgerd"]
