# Data catalog: what to collect, why, and how to store it

Use this when designing any table. Rule zero: **don't collect it if the feature works without it.**

## 1. Classification (decides the storage method)

| Class | Examples | Storage | Access |
|---|---|---|---|
| **P0 Public** | published titles, product names, prices, public profile handle | plain columns | anyone |
| **P1 Internal** | ids, status, counters, timestamps, settings | plain columns | app + staff |
| **P2 Personal** | email, name, phone, postal address, IP, device id, filenames, free text about a person, user content | `_enc` (AES-GCM) + `_bidx` if looked up; IPs truncated; free text encrypted when it is *about* people (notes, messages) | owner + staff with a reason |
| **P3 Sensitive / special category** (GDPR Art. 9/10) | health, biometrics, ethnicity, religion, sex life/orientation, politics, union membership, criminal records, precise location history, children's data | `_enc` with a **separate key id** (`ENCRYPTION_KEYS` entry), explicit consent or other Art. 9 condition, DPIA, access logged in `audit_log` | owner + named roles only |
| **P4 Financial / credentials** | card numbers, bank accounts, government ids, passwords, API secrets | **never store card data** (use Stripe/Adyen tokens); passwords → argon2id; tokens → SHA-256 hash; gov ids → `_enc` + strict retention | system only |

## 2. Lawful basis cheat-sheet (GDPR Art. 6)
- **Contract** – data needed to provide what the user signed up for (account email, order address).
- **Legal obligation** – invoices/tax records (typically 6–10 years depending on country), KYC/AML.
- **Legitimate interest** – security logs, fraud prevention, basic product analytics (do a short LIA, allow objection).
- **Consent** – marketing emails, non-essential cookies, optional profiling, special-category data. Must be
  freely given, specific, recorded (`consents` table), as easy to withdraw as to give.

## 3. By stakeholder (who the data is about)

| Stakeholder | Typical data | Class | Basis | Retention (default) |
|---|---|---|---|---|
| End user / customer | email, name, phone, locale, password hash, sessions | P2/P4 | contract | account lifetime; sessions 30 d; OTP 1 d |
| Visitor (not signed in) | truncated IP in rate-limit memory only; no cookies | P2 | legit. interest | in-memory, minutes |
| Buyer | billing/shipping address, order lines, payment provider ids, invoices | P2/P1 | contract + legal | orders 6–10 y (tax law), addresses with the order |
| Seller / provider (marketplace) | legal name, business id/VAT, payout account (at provider), KYC docs | P2/P4 | contract + legal (DSA trader traceability) | relationship + legal period |
| Employee / staff / admin | work email, role, admin audit trail | P2 | contract / legit. interest | employment + 1 y for audit |
| Support requester | messages, attachments | P2 | contract / legit. interest | 2 y after closure |
| Third parties mentioned in content | names/photos in uploads or messages | P2 | legit. interest of uploader; honour takedown | with the content |
| Children | anything | P3 | parental consent | minimum possible |
| Job applicants (HR apps) | CV, contact, assessment | P2 (+P3) | pre-contract / consent | 6 months after decision (common practice) |
| Patients (health apps) | diagnoses, prescriptions | P3 | explicit consent / health law | per medical law (often 10+ y) |

## 4. By app type – what you usually need (and what to avoid)

| App type | Needed | Avoid / be careful |
|---|---|---|
| **SaaS / productivity** | account email, org/workspace, role, content, billing provider id | tracking keystrokes, collecting phone "just in case" |
| **E-commerce** | email, shipping address, order history, provider payment id, invoice data | storing cards; date of birth unless age-restricted goods; account required for guest checkout |
| **Marketplace** | buyer + seller profiles, listings, messages, reviews, trader info (DSA), payouts via provider | exposing seller home addresses; reviews without moderation process |
| **Social / community** | handle, avatar, posts, follows, reports, blocks | precise location, contact-list upload without consent, minors without safeguards |
| **Booking / scheduling** | name, contact, booking times, provider calendar | free-text notes revealing health (→ P3) |
| **Fintech** | KYC via provider (store result + reference, not documents), transactions, IBAN masked | raw ID scans in your DB; long retention beyond AML law |
| **Health / fitness** | P3 data with explicit consent, clinician accounts | analytics SDKs, ad pixels, sharing with "partners" |
| **EdTech** | student account, progress, teacher roles, parental consent for minors | behavioural profiling of minors |
| **Content / media / blog** | author accounts, comments (email for moderation) | third-party embeds that track (YouTube → use privacy-enhanced mode or click-to-load) |
| **IoT / connected product** | device id, telemetry, owner link; Data Act access rights | indefinite raw telemetry; CRA obligations apply |
| **HR / recruiting** | applicant data, interview notes | AI scoring of candidates (AI Act high-risk) |
| **Internal tools / B2B** | work emails, roles, audit trail | personal emails, unnecessary HR data |

## 5. Field-level template (copy into `app-data-map.md`)

| Table.column | Description | Class | Basis | Retention | Storage | Who can read |
|---|---|---|---|---|---|---|
| users.email_enc | sign-in & service email | P2 | contract | account life | AES-GCM + bidx | owner, support |
| users.phone_enc | optional 2nd factor/contact | P2 | consent/contract | account life | AES-GCM + bidx | owner |
| audit_log.ip_trunc | security forensics | P2 (pseudonymous) | legit. interest | 365 d | truncated /24 | security staff |
| items.title | user content | P1/P2 | contract | until deleted | plain (searchable) | owner (+public if published) |

Searchable free text (e.g. item titles) stays plain so Postgres full-text search works; it is still personal
data if it can identify someone: covered by access control, export, deletion and backup encryption.
