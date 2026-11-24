# Hosting: cheap, simple, scalable later

## Recommended starting setup (~€6–10 / month total)
| Piece | Choice | Cost (Nov 2026, check current prices) |
|---|---|---|
| Server | 1 small VPS, 2 vCPU / 2–4 GB RAM, Ubuntu 24.04, EU region. Hetzner CX/CAX, Netcup, OVH, Contabo… | ~€4–8/mo. Hetzner raised prices twice in 2026 (CX23 ≈ €5.49 + €0.50 IPv4) and cheap plans sell out – compare before ordering |
| TLS, HTTP/3, compression | Caddy (in compose) | free |
| Database | Postgres 18 on the same VPS | included |
| Files | local volume to start → Cloudflare R2 when media grows (no egress fees) | R2: 10 GB free, then $0.015/GB-month |
| Email | Amazon SES ($0.10 / 1,000) or Brevo (300/day free) / Resend (3,000/mo free) via SMTP | ~€0–1 |
| SMS | only if needed (Twilio/Vonage, priced per country) | pay per message |
| CI/CD + image registry | GitHub Actions + GHCR | free tier |
| DNS / CDN (optional) | Cloudflare free plan in front | free |
| Backups | `deploy/backup.sh` → R2/B2 bucket | cents |

Why this is cheap: one Go binary uses ~20–40 MB RAM idle; Postgres replaces Redis, queues and a search
cluster; Caddy replaces a load balancer + certificate service; no Node/SSR servers; images and video are
compressed once and served with year-long cache headers.

## First deploy (step by step)
1. **Server**: create the VPS with your SSH key. As root: `bash server-setup.sh "<your ssh public key>"`.
2. **DNS**: A/AAAA records for `example.com` and `www` → server IP.
3. **GitHub**: push the repo. CI builds and pushes `ghcr.io/<you>/<repo>:latest` on every push to `main`.
   If the package is private, on the server run `docker login ghcr.io` with a read-only token.
4. **Files on the server** (as `deploy`):
   ```bash
   cd /opt/app
   # copy from the repo: deploy/compose.prod.yaml deploy/Caddyfile deploy/backup.sh
   cp .env.example .env    # then edit:
   ```
   Production `.env` differences: `APP_ENV=production`, `BASE_URL=https://example.com`, `COOKIE_SECURE=true`,
   `DOMAIN=example.com`, `ACME_EMAIL=you@example.com`, `APP_IMAGE=ghcr.io/you/repo:latest`,
   `POSTGRES_PASSWORD=<long random>`, `BACKUP_PASSPHRASE=<long random>`, fresh secrets from `make keygen`,
   real SMTP settings. Remove `DATABASE_URL` (compose sets it).
5. `docker compose -f compose.prod.yaml up -d` → Caddy fetches certificates; migrations run on start.
6. Backups: `crontab -e` → `15 3 * * * /opt/app/backup.sh >> /var/log/app-backup.log 2>&1`; configure
   `rclone config` with an `offsite` remote (R2/B2). **Test a restore.**
7. Auto-deploy: in GitHub → Settings → Environments → `production`: secret `DEPLOY_SSH_KEY` (private key for
   `deploy`), variable `DEPLOY_HOST`. Every green push to `main` now deploys (~1 s restart; Caddy retries).
8. Monitoring: free uptime check (e.g. UptimeRobot, Better Stack) on `https://example.com/readyz`.

## Scaling path (do these only when metrics say so)
1. **Bigger VPS** (vertical) – simplest, often enough for 10–100k users.
2. **Media to R2** (`STORAGE_DRIVER=s3`) + `S3_PUBLIC_URL` on a CDN domain: removes file traffic from the VPS.
3. **Separate worker**: run `app worker` as its own service (heavy video encoding no longer slows requests);
   web replicas can use the slim `web` image target.
4. **Managed Postgres** or a dedicated DB server; add a read replica for heavy reports.
5. **More web instances** behind Caddy (`reverse_proxy app1:8080 app2:8080`). The app is stateless
   (sessions in DB/JWT, files in S3, jobs in Postgres). Note: rate limits are per instance (in memory) –
   move them to Caddy or a shared store at that point.

## Operations cheatsheet
```bash
docker compose -f compose.prod.yaml ps | logs -f app | pull && up -d
docker compose -f compose.prod.yaml exec db psql -U app app
docker compose -f compose.prod.yaml run --rm app migrate
```
