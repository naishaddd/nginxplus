// Package nginxpp holds the embedded web UI. The Go code lives in internal/
// and the entry point in cmd/nginxpp.
package nginxpp

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticFiles embed.FS

// StaticFS returns the web UI (index.html, app.css, vendor/…) rooted at static/.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
