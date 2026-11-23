---
name: add-entity
description: Add a new data entity end-to-end (migration, Go module, API, Flutter list/edit screens, tests, docs) by copying the notes reference module. Use whenever a new resource/table/screen is needed.
argument-hint: "<EntityName> <fields and behaviour>"
---

# Add entity: $ARGUMENTS

Copy patterns, don't invent new ones. Reference implementation:
`backend/internal/notes/notes.go`, `backend/migrations/0001_init.sql` (notes table),
`frontend/lib/features/notes/**`.

1. **Migration** `backend/migrations/NNNN_<plural>.sql` (next number):
   `id uuid DEFAULT uuidv7()`, `owner_id … REFERENCES users ON DELETE CASCADE` (if user-owned),
   `version int` (if editable offline), `created_at/updated_at` + `set_updated_at` trigger,
   indexes `(owner_id, <sort col> DESC, id DESC)` for each sort, GIN tsvector `search` if searchable,
   `COMMENT ON COLUMN … 'pii=… basis=… retention=…'` for every personal-data column.
2. **Go module** `backend/internal/<plural>/<plural>.go`: copy notes; adjust struct, `cols`, `listSpec`
   (only whitelisted filters/sorts), validation tags, SQL. Register in `internal/server/server.go`:
   `x.Register(v1.Group("/<plural>", requireAuth, idem), d)`.
3. **Privacy**: add the table to `sections` in `users.export`; add rows to `docs/compliance/DATA_CATALOG.md`.
4. **Tests**: extend `internal/server/server_test.go` flow (create, list with filter, conflict) — run `task test:api`.
5. **Flutter** `frontend/lib/features/<plural>/`: copy notes (repository → controller extends `PagedNotifier` → list page with
   `PagedListView` + `SearchFilterBar` → edit page with `FormBuilder`). Add routes in `app/router.dart` as
   `deferred as`, and a tab in `destinations` if top-level.
6. **Docs**: add endpoints to `docs/API.md`.
7. Verify: `go test ./internal/<plural>/... ./internal/server/...`, `flutter analyze`, `flutter test`.
