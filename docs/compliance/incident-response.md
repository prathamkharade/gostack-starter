# Incident & breach runbook

Keep this short and practical. Name an owner and a backup person.

## 1. Detect & contain (first hour)
- Signals: alerts from uptime monitor, unusual audit_log patterns (`auth.refresh_reuse`, spikes of
  `auth.login_failed`), govulncheck/Dependabot critical advisory, report to security.txt contact.
- Contain: rotate the affected secret (`JWT_SECRET` rotation logs everyone out; revoke sessions with
  `UPDATE sessions SET revoked_at = now()`), block IPs at the firewall/Caddy, take the service to
  maintenance mode if data is leaking, preserve logs and a DB snapshot for forensics.

## 2. Assess
What data, how many people, which countries, is it encrypted (our `_enc` columns + separate keys often
mean "unintelligible" → lower risk), is the vulnerability actively exploited?

## 3. Notify – clocks start when you become aware
| Regime | Who | Deadline |
|---|---|---|
| GDPR Art. 33 | lead supervisory authority | **72 hours** (unless unlikely to result in risk) |
| GDPR Art. 34 | affected users | without undue delay if high risk |
| CRA Art. 14 (product manufacturers, since 11 Sep 2026) | ENISA single reporting platform + CSIRT | **24 h** early warning, 72 h notification, final report 14 days (vuln) / 1 month (incident) |
| NIS2 (if in scope) | national CSIRT/authority | 24 h early warning, 72 h notification, 1 month final |
| India DPDP (from May 2027) | Data Protection Board + affected users | without delay; detailed report within 72 h |
| Contracts | business customers (DPA terms) | usually 24–72 h |

Document every incident (even non-notifiable) in an internal register: facts, effects, remedial action.

## 4. Recover & learn
Patch, redeploy (`git push` → CI → deploy), verify, restore from backup if needed, write a blameless
post-mortem in `docs/decisions/` and add a regression test.
