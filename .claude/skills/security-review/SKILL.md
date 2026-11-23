---
name: security-review
description: Review the current diff (or a named area) for security and privacy defects against this kit's rules. Use before merging auth, data, upload or payment changes.
argument-hint: "[path or 'diff']"
disable-model-invocation: true
context: fork
---

Review $ARGUMENTS (if empty: the current `git diff` plus staged changes) against `docs/SECURITY.md` and `CLAUDE.md` rules. Check:
- Authz: every user-owned query filters by `owner_id`; admin routes use `RequireRole`.
- Injection: no string-built SQL with user input; listquery specs whitelist columns; no shell exec with user input.
- Secrets/PII: nothing logged; no secrets in code or `frontend/env`; new PII columns tagged, exported, catalogued.
- Input: validate tags on every request struct; size limits; uploads never trusted by extension/MIME alone.
- Tokens/cookies unchanged in a weaker way; rate limits on new unauthenticated endpoints.
- Dependencies: new ones justified; run `task lint` (govulncheck).
Report only real issues with file:line, severity, and the fix. No style nits.
