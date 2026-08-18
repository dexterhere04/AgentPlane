// Package migrations embeds the SQL migration files so the server can
// apply them automatically at startup via internal/db.Migrate.
package migrations

import "embed"

// FS holds the *.sql migration files at the root of this directory.
// Callers read the embedded files with fs.ReadDir / fs.ReadFile and apply
// the *.up.sql files in filename order.
//
//go:embed *.sql
var FS embed.FS
