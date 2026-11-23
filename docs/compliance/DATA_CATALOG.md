# Data catalog — what to collect, why, how to store it

Rule zero: **don't collect it unless a v1 feature needs it.** Every field below must map to a purpose and a
lawful basis; if you can't name both, drop the field. Keep this file in sync with column comments
(`task db:pii-report`).

## Storage tiers (pick per field)
| Tier | How | Use for |
|---|---|---|
| T0 plain | normal column | non-personal data, user-chosen display names, content the user publishes |
| T1 protected | plain column + access control + disk encryption + excluded from logs | email, name, address, order history |
| T2 encrypted | `secure.Keyring.Encrypt` (AES-256-GCM) + `Index()` blind index if lookups needed | phone, national IDs, date of birth, precise location history, bank IBAN, tokens from third parties |
| T3 special | T2 + DPIA + explicit consent/legal basis + per-access audit + shortest retention | health, biometrics, religion, sexuality, ethnicity, political views, trade-union, genetic, criminal records, children's data |
| Never store | — | card numbers/CVV (use payment provider tokens), raw passwords (argon2id only), OTPs (HMAC only), government ID images unless legally required (use a KYC provider) |
| Pseudonymise | HMAC/truncate | IP addresses, device IDs, analytics identifiers |

## Platform data (already in the kit)
| Data | Table.column | Tier | Basis | Retention |
|---|---|---|---|---|
| Email | users.email | T1 | contract | account + grace |
| Phone | users.phone_enc/phone_hash | T2 | contract | account + grace |
| Name | users.name | T1 | contract | account + grace |
| Password hash | users.password_hash | argon2id | contract | account |
| Terms/privacy acceptance, consents | users.terms_*, consents | T1 | legal obligation (proof) | account (+ legal hold if disputed) |
| Sessions (UA, IP hash) | sessions | T1/pseudo | legitimate interest: security | session + 30 d |
| Security events | audit_log (ip_hash) | pseudo | legitimate interest: security | AUDIT_RETENTION (1 y) |
| OTP | otp_codes | hashed | contract | 1 d |
| Uploaded files | media + object storage | T1 (T3 if health docs) | contract | until deleted / account |
| Notes (example) | notes | T1 | contract | until deleted / account |

## By stakeholder (who you hold data about)
| Stakeholder | Typical data | Notes |
|---|---|---|
| End users | account, profile, content, usage | core of this catalog |
| Admins/staff | account, role, admin audit trail | MFA mandatory; log every admin read of T2/T3 data |
| Business customers (B2B) | company, VAT ID, billing contact, invoices | invoices must be kept 6–10 y (tax law) → retention overrides erasure; restrict, don't delete |
| Vendors/sellers (marketplaces) | legal name, address, trade register, payout account | DSA trader traceability; payout data via payment provider |
| Third parties mentioned in content | names/photos in uploads | user is responsible; provide report/removal flow |
| Minors | age band, guardian contact | high-privacy defaults, no profiling, parental consent per country |
| Website visitors (not users) | server logs | keep 7–14 d, no analytics without consent |

## By app type — extra fields and how to treat them
| App type | Extra data you'll be tempted to collect | Keep / tier / basis | Watch out |
|---|---|---|---|
| E-commerce | addresses, orders, payment refs, wishlists | T1, contract; tax records 10 y | card data → provider only; price/VAT display, withdrawal right, review authenticity |
| Marketplace | seller KYC, payouts, ratings, messages | KYC via provider (store reference only), T2 for IBAN | DSA trader checks, notice-and-action, ranking transparency |
| Social / community | profile, posts, follows, DMs, location tags | T1; DMs T1+E2E if promised | moderation duties (DSA), minors, deletion of content on erasure |
| SaaS / B2B tool | org, members, billing, workspace content | T1, contract; you're often the *processor* → sign DPA with customers | sub-processor list, data residency options |
| Health / fitness | conditions, metrics, medications, cycle data | **T3**, explicit consent, DPIA | no ad SDKs, export/erasure must cover backups' expiry, HIPAA if US covered entity |
| Fintech / wallets | KYC, transactions, credit data | T2/T3; legal obligation (AML) retention 5–10 y | erasure limited by AML; DORA/PSD2 if regulated |
| Education | student records, grades, minors | T1/T3 for minors; contract with school (often processor) | AI scoring = AI Act high-risk |
| Booking / travel | dates, guests, passport (only if legally required) | passport T2 short retention | don't store passport scans unless required |
| Dating | orientation, photos, location | **T3** (sexual orientation) + precise location T2 | DPIA, safety features, age ≥18 checks |
| Delivery / mobility | live location, routes, driver docs | location T2, minimise frequency/precision, short retention | worker data = employment law |
| Real estate / services | ID docs, income proof | via provider/encrypted short retention | – |
| HR / hiring | CVs, interviews, assessments | T1/T3, legitimate interest/contract, delete after 6 months unless consent | AI screening = AI Act high-risk |
| IoT / smart devices | sensor data, device IDs | T1/T2, Data Act user access rights | CRA security, firmware updates |
| AI features | prompts, outputs, feedback | T1, purpose-limited; opt-in before using for training | AI Act transparency; don't send T3 data to third-party models without basis + DPA |

## Adding a field — checklist
1. Purpose + lawful basis + retention written here.
2. Tier chosen; T2/T3 use `secure.Keyring`.
3. Column comment `pii=… basis=… retention=…` in the migration.
4. Included in `/v1/me/export`; deleted by cascade or explicit purge.
5. Retention job updated if shorter than account lifetime.
6. Privacy policy + store labels updated.
