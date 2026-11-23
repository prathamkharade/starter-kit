# syntax=docker/dockerfile:1
# Two images from one build:
#   --target api     ~15 MB distroless, no shell, non-root
#   --target worker  alpine + ffmpeg/libvips/ghostscript for media compression
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
CMD ["serve"]

FROM alpine:3.22 AS worker
RUN apk add --no-cache ffmpeg vips-tools vips-heif ghostscript ca-certificates tzdata \
 && adduser -D -H -u 10001 app
COPY --from=build /out/app /app
USER 10001
ENTRYPOINT ["/app"]
CMD ["worker"]
