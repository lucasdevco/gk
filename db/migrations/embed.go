package migrations

import "embed"

// FS contains every database migration used at startup.
//
//go:embed *.sql
var FS embed.FS
