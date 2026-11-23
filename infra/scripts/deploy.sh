#!/usr/bin/env bash
# Zero-fuss deploy on the server: pull new images, migrate, restart, prune.
# Usage (on server, in repo root):  TAG=<git-sha|latest> infra/scripts/deploy.sh
# GitHub Actions calls this over SSH after pushing images (see .github/workflows/deploy.yml).
set -euo pipefail
cd "$(dirname "$0")/.."

export TAG=${TAG:-latest}
COMPOSE="docker compose --env-file .env.prod -f compose.prod.yml"

[ -f .env.prod ] || { echo "infra/.env.prod missing (copy .env.prod.example)"; exit 1; }
chmod 600 .env.prod

$COMPOSE pull api worker caddy
$COMPOSE up -d db
# Migrations run in a one-off container before the new API starts (also safe if run twice).
$COMPOSE run --rm --no-deps api migrate
$COMPOSE up -d --remove-orphans
docker image prune -f >/dev/null
$COMPOSE ps
