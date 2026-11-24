# syntax=docker/dockerfile:1.7
# Multi-stage build. Final images:
#   runtime (default): Alpine + ffmpeg + libvips for media processing  (~120 MB)
#   web:               distroless, binary only, for web-only replicas     (~25 MB)

ARG GO_VERSION=1.27
ARG ALPINE_VERSION=3.24

FROM golang:${GO_VERSION}-alpine AS build
ARG TAILWIND_VERSION=4.3.5
ARG TARGETARCH
RUN apk add --no-cache curl \
 && arch=$([ "$TARGETARCH" = "arm64" ] && echo arm64 || echo x64) \
 && curl -fsSL -o /usr/local/bin/tailwindcss \
    "https://github.com/tailwindlabs/tailwindcss/releases/download/v${TAILWIND_VERSION}/tailwindcss-linux-${arch}-musl" \
 && chmod +x /usr/local/bin/tailwindcss
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go tool templ generate \
 && tailwindcss -i static/css/input.css -o static/css/app.css --minify \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

# Slim web-only image (no media tools). Use when the worker runs separately.
FROM gcr.io/distroless/static-debian12:nonroot AS web
COPY --from=build /out/app /app
ENV ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
CMD ["web"]

# Default: web + worker with media tools.
FROM alpine:${ALPINE_VERSION} AS runtime
RUN apk add --no-cache ca-certificates tzdata ffmpeg vips-tools vips-heif \
 && adduser -D -H -u 10001 app \
 && mkdir -p /data/media && chown app:app /data/media
COPY --from=build /out/app /usr/local/bin/app
USER app
ENV ADDR=:8080 STORAGE_LOCAL_DIR=/data/media
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["app", "healthcheck"]
ENTRYPOINT ["app"]
CMD ["all"]
