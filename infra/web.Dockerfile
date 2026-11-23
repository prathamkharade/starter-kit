# syntax=docker/dockerfile:1
# Builds the Flutter web app and bakes it into a Caddy image (one container
# serves HTTPS + static app + API proxy). Build context: repo root.
#   docker build -f infra/web.Dockerfile -t app/web .
FROM ghcr.io/cirruslabs/flutter:stable AS build
WORKDIR /app
COPY frontend/pubspec.yaml frontend/pubspec.lock* ./
RUN flutter pub get
COPY frontend/ ./
ARG APP_VERSION=1.0.0
# --wasm: Skwasm renderer where supported (faster), automatic JS fallback elsewhere.
# --no-web-resources-cdn: self-host CanvasKit/fonts (no Google CDN requests).
RUN flutter build web --release --wasm --no-web-resources-cdn \
      --dart-define=ENV=prod --dart-define=APP_VERSION=${APP_VERSION} \
 && find build/web -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.wasm' -o -name '*.json' -o -name '*.html' -o -name '*.css' -o -name '*.otf' -o -name '*.ttf' -o -name '*.svg' \) \
      -exec gzip -9 -k {} \;

FROM caddy:2-alpine AS brotli
RUN apk add --no-cache brotli
COPY --from=build /app/build/web /srv
RUN find /srv -type f \( -name '*.js' -o -name '*.mjs' -o -name '*.wasm' -o -name '*.json' -o -name '*.html' -o -name '*.css' -o -name '*.otf' -o -name '*.ttf' -o -name '*.svg' \) \
      -exec brotli -q 11 -k {} \;

FROM caddy:2-alpine
COPY infra/Caddyfile /etc/caddy/Caddyfile
COPY --from=brotli /srv /srv
