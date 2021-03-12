package migrations

import "embed"

//go:embed counter/*.sql
var CounterMigrations embed.FS
