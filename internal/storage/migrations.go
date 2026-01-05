package storage

import "embed"

//go:embed all:migrations
var migrationsFS embed.FS
