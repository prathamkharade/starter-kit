// Package notes is the REFERENCE MODULE. To add an entity, copy this folder,
// rename, adjust the SQL + Spec, add a migration, and register it in
// internal/server/server.go. It demonstrates: ownership checks, validation,
// search/filter/sort/cursor pagination, optimistic concurrency (version →
// 409 for offline-sync conflicts), and idempotent writes.
package notes

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"app/internal/platform/httpx"
	"app/internal/platform/kit"
	"app/internal/platform/listquery"
)

type Note struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags"`
	Archived  bool      `json:"archived"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const cols = `id, title, body, tags, archived, version, created_at, updated_at`

// listSpec whitelists what clients may filter/sort/search on.
var listSpec = listquery.Spec{
	Filters: map[string]listquery.Field{
		"archived":   {Column: "archived", Type: listquery.Bool},
		"tags":       {Column: "tags", Type: listquery.TextArray},
		"created_at": {Column: "created_at", Type: listquery.Time},
	},
	Sorts: map[string]listquery.Field{
		"updated_at": {Column: "updated_at", Type: listquery.Time},
		"created_at": {Column: "created_at", Type: listquery.Time},
		"title":      {Column: "title", Type: listquery.Text},
	},
	DefaultSort:  "-updated_at",
	SearchColumn: "search",
	FuzzyColumn:  "title",
}

type Module struct{ d *kit.Deps }

func Register(r fiber.Router, d *kit.Deps) {
	m := &Module{d}
	g := r // mounted at /v1/notes by server.go
	g.Get("/", m.list)
	g.Post("/", m.create)
	g.Get("/:id", m.get)
	g.Patch("/:id", m.update)
	g.Delete("/:id", m.delete)
}

func scan(row pgx.Row) (Note, error) {
	var n Note
	err := row.Scan(&n.ID, &n.Title, &n.Body, &n.Tags, &n.Archived, &n.Version, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, httpx.ErrNotFound
	}
	return n, err
}

func (m *Module) list(c fiber.Ctx) error {
	q, err := listquery.Parse(c.Query, listSpec)
	if err != nil {
		return httpx.Err(400, "invalid_query", err.Error())
	}
	sql, args := q.SQL("SELECT "+cols+" FROM notes", "owner_id = $1", httpx.UserID(c))
	rows, err := m.d.DB.Query(c, sql, args...)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Note, error) { return scan(r) })
	if err != nil {
		return err
	}
	return c.JSON(listquery.MakePage(q, items, func(n Note) (any, string) {
		switch q.SortColumn() {
		case "created_at":
			return n.CreatedAt, n.ID.String()
		case "title":
			return n.Title, n.ID.String()
		default:
			return n.UpdatedAt, n.ID.String()
		}
	}))
}

type input struct {
	Title    *string   `json:"title" validate:"omitempty,min=1,max=200"`
	Body     *string   `json:"body" validate:"omitempty,max=50000"`
	Tags     *[]string `json:"tags" validate:"omitempty,max=20,dive,min=1,max=40"`
	Archived *bool     `json:"archived"`
	Version  *int      `json:"version"` // required on PATCH for conflict detection
}

func (m *Module) create(c fiber.Ctx) error {
	var in input
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.Title == nil {
		return &httpx.APIError{Status: 422, Code: "validation_failed", Message: "Title is required", Fields: map[string]string{"title": "required"}}
	}
	body, tags := deref(in.Body, ""), deref(in.Tags, []string{})
	n, err := scan(m.d.DB.QueryRow(c, `INSERT INTO notes(owner_id, title, body, tags) VALUES ($1,$2,$3,$4) RETURNING `+cols,
		httpx.UserID(c), *in.Title, body, tags))
	if err != nil {
		return err
	}
	return c.Status(201).JSON(n)
}

func (m *Module) get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return httpx.ErrNotFound
	}
	n, err := scan(m.d.DB.QueryRow(c, `SELECT `+cols+` FROM notes WHERE id=$1 AND owner_id=$2`, id, httpx.UserID(c)))
	if err != nil {
		return err
	}
	return c.JSON(n)
}

func (m *Module) update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return httpx.ErrNotFound
	}
	var in input
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if in.Version == nil {
		return &httpx.APIError{Status: 422, Code: "validation_failed", Message: "version is required", Fields: map[string]string{"version": "required"}}
	}
	n, err := scan(m.d.DB.QueryRow(c, `UPDATE notes SET
			title = coalesce($4, title), body = coalesce($5, body),
			tags = coalesce($6, tags), archived = coalesce($7, archived), version = version + 1
		WHERE id=$1 AND owner_id=$2 AND version=$3 RETURNING `+cols,
		id, httpx.UserID(c), *in.Version, in.Title, in.Body, in.Tags, in.Archived))
	if errors.Is(err, httpx.ErrNotFound) {
		// Distinguish "gone" from "someone else changed it" so the client can resolve the conflict.
		var cur Note
		if cur, err = scan(m.d.DB.QueryRow(c, `SELECT `+cols+` FROM notes WHERE id=$1 AND owner_id=$2`, id, httpx.UserID(c))); err == nil {
			return c.Status(409).JSON(fiber.Map{"error": httpx.ErrConflict, "current": cur})
		}
	}
	if err != nil {
		return err
	}
	return c.JSON(n)
}

func (m *Module) delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return httpx.ErrNotFound
	}
	tag, err := m.d.DB.Exec(c, `DELETE FROM notes WHERE id=$1 AND owner_id=$2`, id, httpx.UserID(c))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return c.SendStatus(204)
}

func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
