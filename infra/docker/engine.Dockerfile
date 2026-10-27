# syntax=docker/dockerfile:1.7
# Image engine: build statis Go, dijalankan di distroless non-root.
# Build dari root repo: docker build -f infra/docker/engine.Dockerfile -t <image> .

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY apps/engine apps/engine
COPY libs/go libs/go

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/engine ./apps/engine/cmd/engine

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/engine /engine
ENV ENGINE_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=5 CMD ["/engine", "healthcheck"]
ENTRYPOINT ["/engine"]
