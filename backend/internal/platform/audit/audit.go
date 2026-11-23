// Package audit records security-relevant events (logins, password changes,
// exports, deletions…). It is your evidence for GDPR Art. 5(2)
// accountability, Art. 33 breach investigation and CRA incident reporting.
// Never put raw personal data in Meta — ids and enums only.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"app/internal/platform/db"
)

// Action names: <area>.<event>. Keep this list in sync with docs/SECURITY.md.
const (
	UserRegistered      = "user.registered"
	LoginSucceeded      = "auth.login_succeeded"
	LoginFailed         = "auth.login_failed"
	AccountLocked       = "auth.account_locked"
	SessionReuse        = "auth.refresh_reuse_detected"
	PasswordReset       = "auth.password_reset"
	LoggedOutEverywhere = "auth.logout_all"
	ConsentChanged      = "privacy.consent_changed"
	DataExported        = "privacy.data_exported"
	DeletionRequested   = "privacy.deletion_requested"
	DeletionCancelled   = "privacy.deletion_cancelled"
	UserPurged          = "privacy.user_purged"
)

func Log(ctx context.Context, q db.Querier, log *slog.Logger, userID uuid.UUID, action string, ipHash []byte, meta map[string]any) {
	var uid any
	if userID != uuid.Nil {
		uid = userID
	}
	if meta == nil {
		meta = map[string]any{}
	}
	m, _ := json.Marshal(meta)
	if _, err := q.Exec(ctx, `INSERT INTO audit_log(user_id, action, ip_hash, meta) VALUES ($1,$2,$3,$4)`,
		uid, action, ipHash, m); err != nil {
		log.Error("audit write failed", "action", action, "err", err) // never block the request on audit
	}
}
