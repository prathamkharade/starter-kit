# Deployment

## One-time
1. **VPS** (Ubuntu 24.04+/Debian 12+, EU region for EU users), add your SSH key, then as root:
   `bash infra/scripts/server-setup.sh` (Docker Engine, firewall, fail2ban, auto-updates, swap, `deploy` user).
2. **DNS** at Cloudflare: `A app.example.com → VPS IP`, proxied (orange cloud). SSL mode **Full (strict)**.
3. **Repo on server** (as `deploy`): add a read-only deploy key on GitHub, `git clone … ~/app`.
4. **Config**: `cp infra/.env.prod.example infra/.env.prod`, fill it; generate keys with
   `docker run --rm ghcr.io/<you>/app/api:latest secrets` (or `task secrets` locally). Store a copy of
   `DATA_ENC_KEY`/`DATA_INDEX_KEY` in your password manager — losing them makes encrypted fields unreadable.
5. **Storage**: R2 buckets + CORS (see `docs/MEDIA.md`). **Email**: verify your domain (SPF, DKIM, DMARC).
6. **GitHub secrets**: `SSH_HOST`, `SSH_USER=deploy`, `SSH_KEY`. Create the `production` environment
   (optionally require approval).
7. **Backups**: `rclone config` (remote `backup` → R2/B2), set `BACKUP_AGE_RECIPIENT` in `.env.prod`,
   add the cron line from `infra/scripts/backup.sh`. **Test a restore.**

## Every release
`git tag v1.2.3 && git push --tags` → Actions builds api/worker/web images → SSH → `infra/scripts/deploy.sh`
(pull, migrate, restart). Rollback: `TAG=<previous sha> infra/scripts/deploy.sh` (migrations are forward-only:
write them so the previous release still works — add columns, don't rename/drop in the same release).

## Mobile stores
- Android: `task build:apk` (AAB), sign with an upload key (keep `key.properties` out of git), Play Console.
  Data safety form ← `docs/compliance/DATA_CATALOG.md`.
- iOS: Xcode archive on a Mac (or Codemagic/GitHub macOS runners), App Store Connect privacy "nutrition label"
  ← same catalog. Both stores require in-app account deletion (already in Settings).
- Force update: raise `APP_MIN_VERSION` (served by `/v1/app/config`).

## Operations
- Logs: `docker compose -f infra/compose.prod.yml logs -f api` (JSON). Health: `/readyz` via an uptime monitor.
- Resource view: `docker stats`. Slow queries: Postgres logs statements > 500 ms.
- Upgrade Postgres major: dump/restore into a new volume (test on a copy first).
