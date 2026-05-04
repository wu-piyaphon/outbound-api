package migrations

import "embed"

// FS holds all migration SQL files embedded at compile time, so the binary is
// self-contained and can be deployed without a separate migrations directory.
//
//go:embed *.sql
var FS embed.FS
