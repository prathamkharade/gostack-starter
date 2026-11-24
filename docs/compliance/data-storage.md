# How personal data is stored, used and destroyed

## Encryption at rest (application level)
`internal/security.Keyring` – AES-256-GCM, random 96-bit nonce per value, ciphertext bound to its column
with AAD (`"users.email"`), key id prefix for rotation. Disk/volume encryption at the host is a bonus; this
layer also protects against SQL injection dumps, leaked backups and over-curious DB access.

```go
enc, _ := keys.Encrypt(email, "users.email")       // store in users.email_enc (bytea)
email, _ := keys.Decrypt(row.EmailEnc, "users.email")
```

**Lookup without decrypting everything**: blind index = HMAC-SHA256(BLIND_INDEX_KEY, scope, normalised value)
stored in `_bidx` with a UNIQUE index. Equality only (no LIKE). Normalise first (`NormalizeEmail/Phone`).

**Key management**
- Generate: `make keygen`. Keep `ENCRYPTION_KEYS`, `BLIND_INDEX_KEY`, `JWT_SECRET` in a password manager or
  secrets vault, **separate from DB backups** (a backup + keys together = plaintext).
- Rotate encryption key: prepend `v2:<newkey>,` to `ENCRYPTION_KEYS`, deploy (new writes use v2), run a
  re-encrypt job (`security.KeyID(blob) != active` → decrypt + encrypt), then remove v1.
- Rotating `BLIND_INDEX_KEY` requires recomputing all `_bidx` columns (decrypt → re-index) – plan downtime-free
  by writing both old/new index columns during migration.
- Lost keys = encrypted data is unrecoverable. That is also a feature: crypto-shredding.

## Hashing
- Passwords: argon2id (m=19 MiB, t=2, p=1), PHC string, auto-rehash on login when params increase.
- Refresh tokens, magic links: SHA-256 (they are 256-bit random; slow hashing is unnecessary).
- OTP codes: HMAC bound to purpose+target; max 5 attempts, 10 min TTL, 5 sends/hour/target.

## Minimisation in logs and telemetry
- Access log: method, path (no query string), status, duration, request id. No IPs, no user ids.
- Audit log: action + ids + truncated IP (/24 IPv4, /48 IPv6). `meta` never contains PII.
- Caddy access log disabled by default. If enabled, set rotation ≤ 14 days and state it in the privacy notice.
- Error messages to users never echo emails/phones back in full (`MaskEmail`, `MaskPhone`).

## Retention (automatic, hourly – `privacy.Retention`)
| Data | Kept |
|---|---|
| OTP codes | 1 day after expiry |
| Sessions | 7 days after expiry/revocation |
| Finished jobs | `RETENTION_JOBS_DAYS` (7) – payload is wiped immediately when the job succeeds |
| Audit log | `RETENTION_AUDIT_DAYS` (365) |
| Failed uploads | marked failed after 1 day |
| Backups | 14 days local, 35 days off-site (`deploy/backup.sh`) |
Add rows here (and a DELETE in `Retention`) for every new data type with a limited lifetime.

## Data-subject requests
- **Access/portability**: `GET /account/export` → JSON of account, consents, sessions, security log and every
  registered exporter. Machine-readable, delivered instantly (deadline is 1 month).
- **Rectification**: profile form. **Erasure**: `POST /account/delete` → transaction deletes the user, cascades
  to owned rows, queues file deletion, pseudonymises audit rows (actor_id → NULL), sends confirmation.
- Backups still contain deleted data until they expire (≤ 35 days) – state this in the privacy notice; on restore,
  re-apply deletions (keep a list of deleted user ids *hashed* in a `deletion_tombstones` table if you need this).
- **Objection / restriction**: handle via support; add a `restricted_at` column to users if you need it.

## Files
- EXIF/GPS and other metadata stripped on processing (images, video, audio). Originals deleted unless `MEDIA_KEEP_ORIGINAL`.
- Private files: served only after an access check (local) or via 15-minute presigned URLs (S3/R2).
- Per-owner de-duplication by SHA-256 (no cross-user dedupe: that would leak whether someone else has a file).

## Where data lives (record this in your ROPA)
| Store | Contains | Location |
|---|---|---|
| Postgres (VPS) | everything structured | your VPS region (choose EU for EU users) |
| Media volume / bucket | uploads | VPS or R2/B2 (pick EU jurisdiction / location hint) |
| Email provider | recipient address, message content (codes) | provider's region – sign DPA |
| SMS provider | phone number, message | provider – sign DPA |
| Backups | encrypted DB + media | VPS + off-site bucket |
