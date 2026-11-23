# Compliance guide

> Engineering guidance, not legal advice. The kit implements the technical controls; you still need a
> privacy policy, terms, and (for sensitive domains) a lawyer/DPO review. Laws checked November 2026.

Design rule: **build to the EU baseline (strictest common denominator), then add market/sector deltas.**
GDPR-grade engineering also satisfies most of UK GDPR, Switzerland's FADP, Brazil's LGPD, India's DPDP Act,
California CCPA/CPRA and similar laws, with the deltas noted below.

## 1. Laws by trigger
| Law | Applies when | Key engineering duties | Kit status |
|---|---|---|---|
| **GDPR** (EU/EEA) | you process personal data of people in the EU | lawful basis per purpose, minimisation, transparency, data-subject rights, security, breach notice 72 h, records (RoPA), DPIA for high risk, processors under DPA, transfer safeguards | ✅ consent ledger, export, erasure w/ grace, retention jobs, audit, encryption, PII-tagged schema, EU hosting |
| **ePrivacy / cookie rules** | storing/reading anything on a user's device | consent for non-essential cookies/storage/SDKs | ✅ only strictly-necessary storage by default (session cookie, cache); analytics/crash are opt-in toggles |
| **EU Accessibility Act** (since 28 Jun 2025) | consumer e-commerce, banking, transport, e-books, communications… (micro-enterprises <10 staff & <€2M exempt for services) | EN 301 549 → WCAG 2.1 AA (2.2 coming); accessibility statement | 🟡 accessible defaults; you test & publish the statement |
| **Cyber Resilience Act** | you place software ("product with digital elements", incl. apps) on the EU market | **reporting since 11 Sep 2026** (24 h/72 h); full security requirements from 11 Dec 2027: secure by default, vuln handling, SBOM, updates for support period | 🟡 secure defaults, Dependabot, incident runbook; you add SBOM (`syft`) + disclosure policy |
| **NIS2** | medium/large orgs in listed sectors (digital infra, health, finance…) | risk management, 24 h incident early warning | runbook in SECURITY.md |
| **Digital Services Act** | you host user content / run a platform or marketplace | notice-and-action, statement of reasons, trader traceability (marketplaces), transparency on recommender systems, no dark patterns | ❌ add a report-content flow when you host UGC |
| **AI Act** (Digital Omnibus on AI adjusted timelines in 2026) | you put AI features in the product | transparency: tell users they're talking to AI, label generated content; high-risk uses (hiring, credit, education scoring, biometrics) carry heavy duties | ❌ add disclosures; avoid high-risk uses without counsel |
| **Data Act** (applies since Sep 2025) | connected products/IoT data, cloud switching | user access to device data, switching/export | export endpoint is a start |
| **PSD2/SCA + PCI DSS** | payments | never touch card data; use hosted checkout (Stripe/Adyen/Mollie) → PCI SAQ-A | ❌ integrate provider when needed |
| **Consumer law** (EU CRD/UCPD, Omnibus) | selling to consumers | clear prices incl. VAT, 14-day withdrawal (digital: waiver checkbox), easy subscription cancellation ("cancel button"), genuine reviews | product decisions |
| **Children** (GDPR Art. 8: 13–16 by country; UK Children's Code; US COPPA <13; DSA minors) | users may be minors | age gate, parental consent, high-privacy defaults, no profiling ads | ❌ add age gate if relevant |
| **Health data** (GDPR Art. 9; US HIPAA; EHDS) | health, biometrics, genetics | explicit consent / legal basis, DPIA, encryption at field level, access logging, BAAs (US) | 🟡 field encryption + audit exist; extend per DPIA |
| **Financial** (AML/KYC, DORA for EU financial entities) | wallets, lending, crypto | KYC via provider, transaction monitoring, record retention 5–10 y (overrides erasure) | ❌ |
| **US state privacy** (CCPA/CPRA + ~20 states) | US users over thresholds | notice at collection, "Do not sell/share", honour Global Privacy Control, sensitive-data limits | 🟡 no selling by design; add GPC handling if you add ads |
| **India DPDP Act 2023** + Rules | Indian users | notice + consent manager concept, children <18 need verifiable parental consent, breach to Board & users | 🟡 consent ledger, erasure; add parental flow |
| **Data localisation/transfers** | data leaves the EU (US SaaS, support tools) | SCCs / EU-US Data Privacy Framework, transfer impact assessment | ✅ default stack can be all-EU (EU VPS, R2 EU jurisdiction, EU email region) |
| **App stores** | publishing apps | privacy labels, in-app account deletion, permission justifications | ✅ deletion in app; labels from DATA_CATALOG |

## 2. Principles → where they live in code
| Principle | Implementation |
|---|---|
| Lawful basis per purpose | `consents` (append-only ledger w/ policy version); contract basis for core data; toggles for optional purposes (default OFF) |
| Data minimisation | IPs as truncated HMAC; OTP destinations hashed; job payloads wiped after send; EXIF/GPS stripped from photos; phone masked in API |
| Storage limitation | `users.Retention` hourly: OTPs 1 d, sessions 30 d after end, idempotency 24 h, audit `AUDIT_RETENTION`, jobs 7 d, deleted users after grace |
| Security (Art. 32) | see SECURITY.md; field encryption via `secure.Keyring` |
| Right of access/portability | `GET /v1/me/export` (JSON) |
| Rectification | `PATCH /v1/me` (+ entity edits) |
| Erasure | `DELETE /v1/me` → 30-day grace → `Purge` deletes objects then rows (cascade); `app purge-user` for support |
| Withdraw consent | Settings toggles → `PUT /v1/me/consents` |
| Accountability | `audit_log`; machine-readable data map via column comments → `task db:pii-report` |
| Privacy by default | analytics/crash/marketing off; no third-party SDKs; no Google Fonts/CDN calls on web |
| Breach readiness | SECURITY.md runbook, audit trail, backups |

## 3. Your documents (templates to fill — keep in this folder)
1. **Privacy policy** (Art. 13): who you are, contact/DPO, purposes + basis (copy from DATA_CATALOG), recipients
   (processors list below), transfers, retention table, rights + how to exercise (in-app), complaint right, cookies/storage.
2. **Terms of service**; **imprint** (Impressum) if serving Germany/Austria.
3. **Records of processing (RoPA, Art. 30)**: export `task db:pii-report` + DATA_CATALOG.
4. **DPIA** when: special-category data at scale, minors, systematic monitoring, location tracking, AI decisions.
5. **Processors & DPAs**: hosting (VPS), Cloudflare, R2, email provider, SMS provider, error tracking — sign each DPA.
6. **Accessibility statement** (EAA). **security.txt** + vulnerability disclosure policy (CRA).
7. **Incident register** (even for breaches you didn't notify).

## 4. Launch checklist
- [ ] Hosting/storage/email in EU regions, DPAs signed, transfers documented
- [ ] Privacy policy + terms URLs set (`frontend/env/prod.env`), versions bumped (`TERMS_VERSION`, `PRIVACY_VERSION`)
- [ ] Every PII column tagged, exported, catalogued (`/ship-check`)
- [ ] Consent purposes match what you actually do; nothing optional runs before opt-in
- [ ] Retention periods set and stated in the policy; backup expiry stated
- [ ] Age policy decided (gate if minors possible)
- [ ] DPIA done if triggered; accessibility tested; security headers A+
- [ ] Incident contacts (authority, CSIRT/ENISA SRP, providers) written in SECURITY.md
- [ ] Store privacy labels filled from DATA_CATALOG
