# syntax=docker/dockerfile:1

# ---- build ----
# CGO is disabled so the result is a static binary that runs in distroless.
FROM golang:1.27-alpine AS build

ARG VERSION=dev
WORKDIR /src

# 依赖先落地，源码改动不会反复下载 module。
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/memalyze ./cmd/memalyze

# ---- runtime ----
# distroless/static: no shell, no package manager, runs as nonroot.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/memalyze /usr/local/bin/memalyze

ENV MEMALYZE_DB=/data/memalyze.db \
    MEMALYZE_HTTP_ADDR=:8080 \
    MEMALYZE_REDACT_MODE=enforce \
    MEMALYZE_AUDIT=on \
    MEMALYZE_ACTOR=container

VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/memalyze"]
