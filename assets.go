// Package assets embeds the web templates and static files so the compiled
// binary is fully self-contained — no working-directory or repo checkout needed.
package assets

import "embed"

// FS holds web/templates and web/static. app.css must exist at build time
// (it's committed), so run `make css` before building if it's missing.
//
//go:embed web
var FS embed.FS
