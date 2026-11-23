# Testing

| Layer | Tool | Where | Run |
|---|---|---|---|
| Go unit | `testing` + testify | `backend/**/_test.go` (secure, listquery, notify…) | `task test:api:unit` |
| API end-to-end | Fiber `app.Test` + real Postgres | `backend/internal/server/server_test.go` | `task test:api` (needs `task infra:up`) |
| Flutter unit/widget | flutter_test + mocktail | `frontend/test/` | `task test:web` |
| Flutter integration | integration_test | `frontend/integration_test/` | `task test:e2e` (device/emulator); web: `task test:e2e:web` (real API + Mailpit; chromedriver matching your Chrome on :4444) |
| Load | k6 (Docker, no install) | `infra/k6/smoke.js` | `task test:load` |
| Static / vulns | go vet, govulncheck, flutter analyze, gitleaks | CI | `task lint` |
| Security headers | securityheaders.com, SSL Labs | prod URL | manual, per release |
| Accessibility | TalkBack/VoiceOver, keyboard on web, `meetsGuideline` in widget tests | – | per feature |

Rules: every bug fix gets a failing test first; every new endpoint gets a line in the e2e flow; CI must be green
to deploy. Mock HTTP in Flutter tests by overriding `dioProvider` in `ProviderScope(overrides: [...])`.
