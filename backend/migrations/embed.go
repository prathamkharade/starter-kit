// Package migrations embeds forward-only SQL migrations.
// Add new files as NNNN_description.sql. Never edit a migration that has shipped.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
