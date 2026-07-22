// Package webui embeds the built Mini App (Vite output under dist/) so it ships
// inside the Go binary. go:embed cannot reach across directories, hence this
// file lives in web/ next to dist/.
//
// dist/ is produced by `npm run build` (or `make web-build`) and is gitignored
// except for a .gitkeep placeholder, so a fresh checkout still compiles — the
// server then serves a "not built" notice until the frontend is built.
package webui

import "embed"

//go:embed all:dist
var DistFS embed.FS
