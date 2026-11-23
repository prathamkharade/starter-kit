// Package media: direct-to-storage uploads with presigned URLs, then
// background compression (see processor.go and docs/MEDIA.md).
//
// Flow: POST /media/uploads → client PUTs bytes to upload_url →
// POST /media/:id/complete → worker compresses → status "ready".
package media

import (
	"errors"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"app/internal/platform/httpx"
	"app/internal/platform/jobs"
	"app/internal/platform/kit"
	"app/internal/platform/listquery"
)

type Variant struct {
	Key   string `json:"key"`
	Width int    `json:"w,omitempty"`
	Bytes int64  `json:"bytes"`
	Type  string `json:"type"`
}

type Media struct {
	ID           uuid.UUID         `json:"id"`
	Kind         string            `json:"kind"`
	Status       string            `json:"status"`
	Visibility   string            `json:"visibility"`
	OriginalName string            `json:"original_name"`
	ContentType  string            `json:"content_type"`
	SizeBytes    int64             `json:"size_bytes"`
	StoredBytes  int64             `json:"stored_bytes"`
	CreatedAt    time.Time         `json:"created_at"`
	URL          string            `json:"url,omitempty"`
	Variants     map[string]string `json:"variants,omitempty"` // name → URL
	storageKey   string
	variants     map[string]Variant
}

const cols = `id, kind, status, visibility, original_name, content_type, size_bytes, stored_bytes, created_at, storage_key, variants`

var listSpec = listquery.Spec{
	Filters: map[string]listquery.Field{
		"kind":   {Column: "kind", Type: listquery.Text},
		"status": {Column: "status", Type: listquery.Text},
	},
	Sorts:       map[string]listquery.Field{"created_at": {Column: "created_at", Type: listquery.Time}},
	DefaultSort: "-created_at",
}

type Module struct{ d *kit.Deps }

func Register(r fiber.Router, d *kit.Deps) {
	m := &Module{d}
	g := r // mounted at /v1/media by server.go
	g.Use(func(c fiber.Ctx) error {
		if d.Store == nil {
			return httpx.Err(503, "storage_not_configured", "File storage is not configured")
		}
		return c.Next()
	})
	g.Get("/", m.list)
	g.Post("/uploads", m.createUpload)
	g.Post("/:id/complete", m.complete)
	g.Get("/:id", m.get)
	g.Delete("/:id", m.delete)
}

func (m *Module) createUpload(c fiber.Ctx) error {
	var req struct {
		Filename    string `json:"filename" validate:"required,max=255"`
		ContentType string `json:"content_type" validate:"required,max=127"`
		Size        int64  `json:"size" validate:"required,gt=0"`
		Visibility  string `json:"visibility" validate:"omitempty,oneof=private public"`
	}
	if err := httpx.Bind(c, &req); err != nil {
		return err
	}
	if req.Size > m.d.Cfg.MediaMaxBytes {
		return httpx.Err(413, "file_too_large", "File is too large")
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	uid := httpx.UserID(c)
	id := uuid.Must(uuid.NewV7())
	key := "u/" + uid.String() + "/" + id.String() + "/original" + safeExt(req.Filename)
	if _, err := m.d.DB.Exec(c, `INSERT INTO media(id, owner_id, kind, visibility, original_name, content_type, size_bytes, storage_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, uid, KindOf(req.ContentType, req.Filename), req.Visibility,
		path.Base(req.Filename), req.ContentType, req.Size, key); err != nil {
		return err
	}
	url, err := m.d.Store.PresignPut(c, key, 15*time.Minute)
	if err != nil {
		return err
	}
	return c.Status(201).JSON(fiber.Map{
		"id": id, "upload_url": url, "method": "PUT",
		"headers": fiber.Map{"Content-Type": req.ContentType}, "expires_in": 900,
	})
}

func (m *Module) complete(c fiber.Ctx) error {
	mm, err := m.load(c, c.Params("id"), httpx.UserID(c))
	if err != nil {
		return err
	}
	if mm.Status != "pending" {
		return c.JSON(m.present(c, mm))
	}
	info, err := m.d.Store.Stat(c, mm.storageKey)
	if err != nil {
		return httpx.Err(400, "upload_missing", "The file has not been uploaded yet")
	}
	if info.Size > m.d.Cfg.MediaMaxBytes {
		_ = m.d.Store.Delete(c, mm.storageKey)
		_, _ = m.d.DB.Exec(c, `DELETE FROM media WHERE id=$1`, mm.ID)
		return httpx.Err(413, "file_too_large", "File is too large")
	}
	tx, err := m.d.DB.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c) //nolint:errcheck
	if _, err := tx.Exec(c, `UPDATE media SET status='processing', size_bytes=$2 WHERE id=$1`, mm.ID, info.Size); err != nil {
		return err
	}
	if err := jobs.Enqueue(c, tx, kit.JobMediaProcess, map[string]string{"id": mm.ID.String()}, jobs.MaxAttempts(3)); err != nil {
		return err
	}
	if err := tx.Commit(c); err != nil {
		return err
	}
	mm.Status = "processing"
	return c.Status(202).JSON(m.present(c, mm))
}

func (m *Module) get(c fiber.Ctx) error {
	mm, err := m.load(c, c.Params("id"), httpx.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(m.present(c, mm))
}

func (m *Module) list(c fiber.Ctx) error {
	q, err := listquery.Parse(c.Query, listSpec)
	if err != nil {
		return httpx.Err(400, "invalid_query", err.Error())
	}
	sql, args := q.SQL("SELECT "+cols+" FROM media", "owner_id = $1", httpx.UserID(c))
	rows, err := m.d.DB.Query(c, sql, args...)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Media, error) { return scan(r) })
	if err != nil {
		return err
	}
	for i := range items {
		items[i] = m.present(c, items[i])
	}
	return c.JSON(listquery.MakePage(q, items, func(x Media) (any, string) { return x.CreatedAt, x.ID.String() }))
}

func (m *Module) delete(c fiber.Ctx) error {
	mm, err := m.load(c, c.Params("id"), httpx.UserID(c))
	if err != nil {
		return err
	}
	keys := []string{mm.storageKey}
	for _, v := range mm.variants {
		keys = append(keys, v.Key)
	}
	if err := m.d.Store.Delete(c, keys...); err != nil {
		return err
	}
	if _, err := m.d.DB.Exec(c, `DELETE FROM media WHERE id=$1`, mm.ID); err != nil {
		return err
	}
	return c.SendStatus(204)
}

func scan(r pgx.Row) (Media, error) {
	var x Media
	err := r.Scan(&x.ID, &x.Kind, &x.Status, &x.Visibility, &x.OriginalName, &x.ContentType,
		&x.SizeBytes, &x.StoredBytes, &x.CreatedAt, &x.storageKey, &x.variants)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, httpx.ErrNotFound
	}
	return x, err
}

func (m *Module) load(c fiber.Ctx, rawID string, owner uuid.UUID) (Media, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return Media{}, httpx.ErrNotFound
	}
	return scan(m.d.DB.QueryRow(c, `SELECT `+cols+` FROM media WHERE id=$1 AND owner_id=$2`, id, owner))
}

// present attaches short-lived URLs (1h) — or CDN URLs for public media.
func (m *Module) present(c fiber.Ctx, x Media) Media {
	if x.Status != "ready" {
		return x
	}
	public := x.Visibility == "public"
	dl := ""
	if x.Kind != "image" && x.Kind != "video" && x.Kind != "audio" {
		dl = x.OriginalName // force download for documents/archives (no inline rendering of untrusted files)
	}
	x.URL, _ = m.d.Store.URL(c, x.storageKey, public, time.Hour, dl)
	if len(x.variants) > 0 {
		x.Variants = map[string]string{}
		for name, v := range x.variants {
			x.Variants[name], _ = m.d.Store.URL(c, v.Key, public, time.Hour, "")
		}
	}
	return x
}

// KindOf classifies by MIME type, falling back to the extension.
func KindOf(contentType, filename string) string {
	ct := strings.ToLower(contentType)
	ext := strings.ToLower(path.Ext(filename))
	switch {
	case strings.HasPrefix(ct, "image/") || in(ext, ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif", ".avif", ".bmp", ".tif", ".tiff"):
		if ct == "image/svg+xml" || ext == ".svg" {
			return "document" // SVG can carry scripts: never treat as an inline image
		}
		return "image"
	case strings.HasPrefix(ct, "video/") || in(ext, ".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v", ".3gp"):
		return "video"
	case strings.HasPrefix(ct, "audio/") || in(ext, ".mp3", ".wav", ".m4a", ".aac", ".ogg", ".flac", ".opus"):
		return "audio"
	case in(ext, ".zip", ".7z", ".rar", ".gz", ".tgz", ".bz2", ".xz", ".zst", ".tar"):
		return "archive"
	case ct == "application/pdf" || in(ext, ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".txt", ".csv", ".json", ".md", ".rtf"):
		return "document"
	default:
		return "other"
	}
}

func in(s string, list ...string) bool {
	for _, x := range list {
		if s == x {
			return true
		}
	}
	return false
}

func safeExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if len(ext) > 10 {
		return ""
	}
	for _, r := range ext[min(1, len(ext)):] {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return ext
}
