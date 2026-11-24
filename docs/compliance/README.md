# Compliance overview

> Not legal advice. This kit implements the **technical** controls that most laws require and documents
> the **organisational** steps you still must take. Have a lawyer/DPO review before launch, especially for
> health, finance, children, employment or AI-decision use cases. Status as of November 2026.

The EU sets the strictest baseline, so the kit is designed to EU standards; meeting them gets you most of
the way for the UK, US states, India, Brazil, Canada, Australia and others.

## Laws that commonly apply to web/mobile apps

| Law | Applies when | Key obligations | Kit support |
|---|---|---|---|
| **GDPR** (EU/EEA) + UK GDPR | You process personal data of people in the EU/UK, or are established there | Lawful basis, transparency notice, data minimisation, security (Art. 32), data-subject rights (access, rectify, erase, port, object), records of processing (ROPA), processor contracts (DPA), breach notice to authority ≤ 72 h, DPIA for high-risk processing, international transfer safeguards | Encryption at rest, blind indexes, export/delete self-service, consent records, retention jobs, audit log, IP truncation, privacy notice template |
| **ePrivacy Directive** (cookies) | Storing/reading anything on a device | Consent for non-essential cookies/trackers; strictly necessary are exempt | Only necessary cookies; no third-party scripts/fonts; cookie notice page. *The Digital Omnibus may relax cookie consent – still in trilogue negotiation, not law yet.* |
| **EU Accessibility Act** (EAA, applies since 28 June 2025) | Consumer-facing e-commerce, banking, transport, e-books, communications services (micro-enterprises <10 staff & <€2 M exempt for services) | WCAG 2.1/2.2 AA level (EN 301 549), accessibility statement | Semantic components, labels, focus, reduced motion, skip link, axe tests, statement page |
| **Cyber Resilience Act** (CRA) | Selling a "product with digital elements" in the EU (apps/software you distribute, incl. their remote backends) | **Since 11 Sep 2026:** report actively exploited vulnerabilities & severe incidents to ENISA (24 h early warning). **From 11 Dec 2027:** secure-by-design, SBOM, security updates, vulnerability handling | security.txt, dependency scanning (govulncheck, Dependabot), CI SBOM-ready, incident runbook. Pure SaaS websites are mostly covered by NIS2/GDPR instead – check with counsel |
| **NIS2** | Medium/large entities in listed sectors (cloud, digital providers, marketplaces, search, social…) | Risk management, incident reporting (24 h / 72 h / 1 month), management accountability | Logging, backups, hardening, incident runbook |
| **Digital Services Act** (DSA) | Hosting user content / marketplaces / platforms | Notice-and-action, statement of reasons, contact point, T&Cs transparency; more for large platforms | Hook points: content reporting is app-specific (add when you host public user content) |
| **AI Act** | You build or deploy AI systems in the EU | Transparency for chatbots/generated content; high-risk obligations moved to **2 Dec 2027** by the AI Omnibus (in force 27 Jul 2026) | Not included; document AI features in the spec and label AI output |
| **Data Act** (applies since 12 Sep 2025) | Connected products/related services; cloud switching | Data access for users, cloud switching/portability | JSON export; self-hostable stack (no lock-in) |
| **PSD2 / PCI DSS 4.0** | Taking card payments | Never touch card numbers yourself | Use Stripe/Adyen/Mollie hosted checkout; store only their customer/payment ids |
| **Consumer law** (EU CRD, Omnibus Directive) | Selling to consumers | Pre-contract info, 14-day withdrawal, price transparency, cancel subscriptions as easily as sign-up | Terms template; build a "cancel" button next to "subscribe" |
| **India DPDP Act + Rules 2025** | Processing personal data in India / of people in India | Notice + consent (itemised), purpose limits, breach reporting, children's verifiable parental consent, grievance redress. Phased: consent-manager rules from 13 Nov 2026, most obligations from **13 May 2027** | Same controls as GDPR; consent records with versions; export/delete; add grievance contact to privacy notice |
| **US state privacy laws** (CCPA/CPRA + ~20 states) | Thresholds by revenue/volume | Notice at collection, opt-out of sale/sharing, honour Global Privacy Control, sensitive data limits | No selling/sharing by default; honour GPC if you add ads/analytics |
| **COPPA** (US, <13) / GDPR Art. 8 (EU, 13-16) | Services directed at children or with actual knowledge | Parental consent, minimal data, no profiling | Not enabled. If children may use the app: age gate + parental flow (do a DPIA) |
| **HIPAA** (US health), **GLBA** (US finance) | Covered entities / their vendors | BAAs, access controls, audit | Encryption + audit help; you still need BAAs and policies |

## What the kit does for you (technical)
- **Privacy by design & default** (GDPR Art. 25): passwordless sign-up asks only for email; profile fields optional.
- **Security of processing** (Art. 32): AES-256-GCM field encryption with key rotation, argon2id, TLS via Caddy,
  strict CSP, CSRF protection, rate limiting, hardened containers, encrypted backups. See `../security.md`.
- **Rights self-service**: `/account` → export JSON (Art. 15/20), edit profile (Art. 16), delete (Art. 17),
  marketing consent toggle (Art. 7(3) withdrawal), sign out everywhere.
- **Accountability**: append-only `consents` (who agreed to which version, when) and `audit_log`.
- **Storage limitation**: hourly retention job (`internal/privacy`) – OTPs, sessions, jobs, audit rows.
- **No third parties by default**: no CDNs, fonts, analytics or trackers → no transfers, no cookie banner.

## What YOU must do (organisational) → `checklist.md`
Privacy notice & terms text, ROPA, DPAs with every processor (host, email, SMS, storage), transfer
assessment for non-EU processors, DPIA if high-risk, breach & incident process, appoint EU/UK representative
if you are outside the EU and target EU users, DPO if required, accessibility statement, security contact.

## Files in this folder
- `data-catalog.md` – what data apps typically collect, per app type and stakeholder, with classification,
  lawful basis and retention. **Start here when designing a data model.**
- `data-storage.md` – how each class must be stored, accessed, logged, backed up and deleted (with code).
- `checklist.md` – launch checklist.
- `incident-response.md` – breach / vulnerability runbook (GDPR 72 h, CRA 24 h, NIS2).
- `cookies-and-tracking.md` – staying banner-free, and what to do if you add analytics/ads.
