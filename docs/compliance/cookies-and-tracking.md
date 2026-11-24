# Cookies, analytics and third parties

**Default: no consent banner needed.** The kit sets only strictly necessary cookies (sign-in, flash message),
loads no third-party scripts, fonts or CDNs, and stores the theme choice in localStorage on the user's request.

## If you need analytics
Prefer, in order:
1. **Server-side counts** from your own DB (sign-ups, orders) – no tracking needed.
2. **Cookieless, self-hosted analytics** (e.g. Plausible CE, Umami, GoatCounter) on your own domain with IPs
   not stored. Several EU authorities accept these without consent when configured for aggregate statistics
   only; document it in the privacy notice. Add its origin to `script-src`/`connect-src` in `internal/web/server.go`.
3. Anything that sets cookies/IDs across sites (Google Analytics, Meta Pixel, ad SDKs) → **prior opt-in consent**
   with a banner that offers "Reject" as prominently as "Accept", records the choice in `consents`, and loads
   the script only after consent. Also a third-country transfer → check DPF/SCCs.

## Embeds
YouTube/Maps/social embeds set cookies. Use click-to-load placeholders (load the iframe only after a click)
and privacy-enhanced modes (`youtube-nocookie.com`).

## Global Privacy Control (US)
If you ever sell/share data or run targeted ads, treat the `Sec-GPC: 1` header as an opt-out (required in
California and several other states).

## Upcoming
The EU Digital Omnibus proposal would move cookie rules into the GDPR and widen consent exemptions; as of
Nov 2026 the Council has a position and trilogues await Parliament. Re-check before relying on it.
