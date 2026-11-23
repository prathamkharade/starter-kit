#!/usr/bin/env bash
# Nightly encrypted Postgres backup → object storage. Install on the server:
#   crontab -e   →   15 3 * * * /home/deploy/app/infra/scripts/backup.sh >> /home/deploy/backup.log 2>&1
# Requirements (server-setup.sh installs them): zstd, age, rclone (configured remote "backup", e.g. R2/B2).
# Encryption: `age` with a PUBLIC key on the server; keep the private key OFFLINE (password manager).
#   generate once on your laptop: age-keygen -o backup-key.txt   → put the "public key" in BACKUP_AGE_RECIPIENT
# Retention: 14 daily + 8 weekly (deleted data disappears from backups within ~2 months — document this
# in your privacy policy; GDPR erasure does not require rewriting backups if they expire on schedule).
set -euo pipefail
cd "$(dirname "$0")/.."
source .env.prod

: "${BACKUP_AGE_RECIPIENT:?set BACKUP_AGE_RECIPIENT (age public key) in infra/.env.prod}"
REMOTE=${BACKUP_REMOTE:-backup:app-backups}
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
FILE="/tmp/db-$STAMP.sql.zst.age"

docker compose --env-file .env.prod -f compose.prod.yml exec -T db \
  pg_dump -U app -d app --format=plain --no-owner \
  | zstd -q -T0 -10 | age -r "$BACKUP_AGE_RECIPIENT" > "$FILE"

rclone copy "$FILE" "$REMOTE/daily/" --s3-no-check-bucket
[ "$(date -u +%u)" = 7 ] && rclone copy "$FILE" "$REMOTE/weekly/" --s3-no-check-bucket
rm -f "$FILE"

rclone delete "$REMOTE/daily/" --min-age 14d
rclone delete "$REMOTE/weekly/" --min-age 56d
echo "backup ok $STAMP"

# RESTORE (test this quarterly!):
#   rclone copy backup:app-backups/daily/db-XXXX.sql.zst.age .
#   age -d -i backup-key.txt db-XXXX.sql.zst.age | zstd -d | docker compose ... exec -T db psql -U app -d app
