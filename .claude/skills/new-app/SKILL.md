---
name: new-app
description: Turn this starter kit into a specific application from a plain-language idea. Use when the user describes a new app to build on the kit.
argument-hint: "<describe the app idea>"
disable-model-invocation: true
---

# Start a new application from the kit

App idea: $ARGUMENTS

Work in phases. Stop for my approval at the end of phase 2. Keep context small: read only the docs named below.

## Phase 1 — Understand (plan mode, no edits)
1. Read `CLAUDE.md`, `docs/ARCHITECTURE.md`, `docs/compliance/DATA_CATALOG.md`.
2. Interview me with AskUserQuestion (max 2 rounds, ≤4 questions each) about only what changes the build:
   users & roles, core entities, must-have flows for v1, which markets (EU? kids? health/finance data?),
   offline needs, media types, login methods (email code / SMS / password), monetisation.

## Phase 2 — Spec (write `docs/APP.md`, then STOP)
Write `docs/APP.md` (≤150 lines) with:
- One-paragraph product summary; roles; v1 scope and explicit NON-goals.
- Entities table: fields, types, which are personal data (pii category, lawful basis, retention), indexes,
  list filters/sorts/search fields, offline yes/no.
- Screens & navigation (tabs in `frontend/lib/app/router.dart` → `destinations`).
- Compliance delta: which rows of `docs/compliance/COMPLIANCE.md` apply (e.g. special-category data → DPIA,
  minors → age gate, payments → PCI via hosted checkout, AI features → AI Act transparency).
- Build plan: ordered milestones, each ending with a runnable check.
Then ask me to approve or edit the spec.

## Phase 3 — Build, one milestone at a time
For each milestone: implement → run its check → fix → summarise in 3 lines → `/clear`-friendly handoff
(note in `docs/APP.md` under "Progress" what is done and what is next).
- Branding: name in `frontend/env/*.env`, `web/index.html`, `web/manifest.json`, `backend/.env.example` (APP_NAME);
  colour/font in `frontend/lib/app/theme/app_theme.dart`; `flutter create --org` reverse-domain id.
- Entities: use the `/add-entity` skill per entity (copies the `notes` reference module).
- Remove the `notes` example module (backend, frontend, migration stays — add a new migration that drops it)
  only after the first real entity works.
- Consent purposes: adjust `togglePurposes` (backend/internal/users/users.go) + `_purposes` (frontend settings).
- Update docs touched by each change.

## Definition of done (every milestone)
`task test` green, `task lint` clean, PII columns tagged + exported + catalogued, no secrets in code.
