#!/usr/bin/env bash
# Encrypted daily Postgres backup (+ media volume) with local retention and optional
# off-site copy. Install on the server with cron:
#   15 3 * * *  /opt/app/backup.sh >> /var/log/app-backup.log 2>&1
#
# Requires in /opt/app/.env: BACKUP_PASSPHRASE (long random string, store it in your password manager).
# Optional: rclone configured with a remote named "offsite" (e.g. Cloudflare R2 / Backblaze B2).
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a
: "${BACKUP_PASSPHRASE:?set BACKUP_PASSPHRASE in .env}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p backups
db_file="backups/db-${stamp}.dump.enc"
media_file="backups/media-${stamp}.tar.gz.enc"

# 1. database: custom format (compressed, restorable table by table), then AES-256 encrypted
docker compose -f compose.prod.yaml exec -T db pg_dump -U app -Fc app \
  | openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$db_file"

# 2. media volume (skip if you use S3/R2: enable bucket versioning/lifecycle instead)
if [ "${STORAGE_DRIVER:-local}" = "local" ]; then
  docker run --rm -v app_media:/data:ro alpine tar -C /data -czf - . \
    | openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$media_file"
fi

# 3. keep 14 days locally
find backups -name '*.enc' -mtime +14 -delete

# 4. off-site copy (3-2-1 rule). Configure once: rclone config
if command -v rclone >/dev/null && rclone listremotes | grep -q '^offsite:'; then
  rclone copy backups offsite:app-backups --max-age 25h
  rclone delete offsite:app-backups --min-age 35d
fi
echo "backup ok ${stamp}"

# Restore (test this regularly!):
#   openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -pass env:BACKUP_PASSPHRASE -in db-XXXX.dump.enc \
#     | docker compose -f compose.prod.yaml exec -T db pg_restore -U app -d app --clean --if-exists
