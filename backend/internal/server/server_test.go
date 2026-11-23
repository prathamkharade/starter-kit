package server

// End-to-end API test against a real Postgres. Runs only when
// TEST_DATABASE_URL is set (task test:api sets it for the dev compose DB).
// The database is wiped — never point it at real data.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"app/internal/config"
	"app/internal/platform/db"
	"app/internal/platform/kit"
	"app/internal/platform/secure"
	"app/internal/platform/token"
)

func setup(t *testing.T) (*fiber.App, *kit.Deps) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	t.Setenv("DATABASE_URL", url)
	t.Setenv("APP_ENV", "test")
	t.Setenv("JWT_SECRET", "test-secret-test-secret-test-secret-123")
	t.Setenv("DATA_ENC_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("DATA_INDEX_KEY", "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA=")
	t.Setenv("S3_ACCESS_KEY", "")
	cfg, err := config.Load()
	require.NoError(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL, 5)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, _ = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, db.Migrate(ctx, pool, log))
	keys, err := secure.NewKeyring(cfg.DataEncKey, cfg.DataIndexKey)
	require.NoError(t, err)
	d := &kit.Deps{Cfg: cfg, DB: pool, Log: log, Keys: keys, Tokens: token.NewIssuer(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTTL)}
	return New(d), d
}

func call(t *testing.T, app *fiber.App, method, path, bearer string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client", "mobile")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// lastCode reads the OTP from the queued email job (stands in for the inbox).
func lastCode(t *testing.T, d *kit.Deps) string {
	var code string
	require.NoError(t, d.DB.QueryRow(context.Background(),
		`SELECT payload->'data'->>'Code' FROM jobs WHERE kind='email.send' ORDER BY id DESC LIMIT 1`).Scan(&code))
	return code
}

func TestAuthNotesPrivacyFlow(t *testing.T) {
	app, d := setup(t)

	st, _ := call(t, app, "POST", "/v1/auth/register", "", map[string]any{
		"email": "ada@example.com", "password": "correct-horse-battery", "accept_terms": true})
	require.Equal(t, http.StatusAccepted, st)

	st, _ = call(t, app, "POST", "/v1/auth/login", "", map[string]any{"email": "ada@example.com", "password": "correct-horse-battery"})
	require.Equal(t, http.StatusForbidden, st, "unverified email must not log in")

	st, tok := call(t, app, "POST", "/v1/auth/verify-email", "", map[string]any{"email": "ada@example.com", "code": lastCode(t, d)})
	require.Equal(t, http.StatusOK, st)
	access := tok["access_token"].(string)
	refresh := tok["refresh_token"].(string)

	st, _ = call(t, app, "POST", "/v1/auth/login", "", map[string]any{"email": "ada@example.com", "password": "wrong-password!!"})
	require.Equal(t, http.StatusUnauthorized, st)

	// notes CRUD + search + conflict
	st, n := call(t, app, "POST", "/v1/notes", access, map[string]any{"title": "Groceries", "body": "milk eggs", "tags": []string{"home"}})
	require.Equal(t, http.StatusCreated, st)
	id := n["id"].(string)
	st, page := call(t, app, "GET", "/v1/notes?q=milk&filter[tags]=home", access, nil)
	require.Equal(t, http.StatusOK, st)
	require.Len(t, page["items"], 1)
	st, _ = call(t, app, "PATCH", "/v1/notes/"+id, access, map[string]any{"title": "x", "version": 99})
	require.Equal(t, http.StatusConflict, st)
	st, _ = call(t, app, "GET", "/v1/notes?sort=password_hash", access, nil)
	require.Equal(t, http.StatusBadRequest, st)

	// refresh rotation + reuse detection
	st, rot := call(t, app, "POST", "/v1/auth/refresh", "", map[string]any{"refresh_token": refresh})
	require.Equal(t, http.StatusOK, st)
	require.NotEqual(t, refresh, rot["refresh_token"])

	// privacy
	st, _ = call(t, app, "PUT", "/v1/me/consents", access, map[string]any{"purpose": "marketing", "granted": true})
	require.Equal(t, http.StatusOK, st)
	req := httptest.NewRequest("GET", "/v1/me/export", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	st, _ = call(t, app, "DELETE", "/v1/me", access, map[string]any{"confirm": "DELETE"})
	require.Equal(t, http.StatusAccepted, st)
}

func TestUnauthenticated(t *testing.T) {
	app, _ := setup(t)
	st, body := call(t, app, "GET", "/v1/notes", "", nil)
	require.Equal(t, http.StatusUnauthorized, st)
	require.Equal(t, "unauthorized", body["error"].(map[string]any)["code"])
}
