package migrations

import "embed"

// Files contains the versioned SQL migrations.
//
// Only canonical version-prefixed files may enter the migration filesystem.
// This excludes macOS AppleDouble sidecars such as ._00001_initial.sql.
//
//go:embed [0-9]*.sql
var Files embed.FS
