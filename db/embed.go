// Package db embeds the goose SQL migrations so they ship inside the binary
// (go:embed cannot reach across directories, hence this file lives next to the
// migrations it embeds).
package db

import "embed"

// MigrationsFS holds the goose migration files under migrations/.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
