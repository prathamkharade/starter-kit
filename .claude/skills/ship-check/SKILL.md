---
name: ship-check
description: Pre-release checklist — tests, lint, compliance items, prod config, store readiness. Use before tagging a release.
disable-model-invocation: true
---

Run and report a pass/fail table:
1. `task test`, `task test:api`, `task lint`, `task build:web` — all green.
2. `task db:pii-report` matches `docs/compliance/DATA_CATALOG.md` (every tagged column documented and exported).
3. `infra/.env.prod.example` lists every variable in `backend/internal/config/config.go`.
4. `docs/compliance/COMPLIANCE.md` → "Launch checklist": mark each item done / not applicable / TODO.
5. Versions: bump `frontend/pubspec.yaml` version, `APP_LATEST_VERSION`; decide `APP_MIN_VERSION` (force update).
6. Changelog entry in `docs/APP.md` → "Releases".
List blockers first.
