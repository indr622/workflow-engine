# syntax=docker/dockerfile:1.7
# Image worker-example: demo SDK Go, build statis, distroless non-root (pola sama dengan engine).
# Build dari root repo: docker build -f infra/docker/worker.Dockerfile -t <image> .

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY apps/worker-example apps/worker-example
COPY libs/go libs/go

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/worker ./apps/worker-example/cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/worker /worker
USER nonroot:nonroot
ENTRYPOINT ["/worker"]
