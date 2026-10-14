// Package web embeds the dry-run UI that internal/server serves at the
// application root. Go cannot embed files outside the embedding package's
// directory, which is why the assets live here rather than beside the server.
package web

import (
	"embed"
	"net/http"
)

//go:embed *.html *.js *.css
var assets embed.FS

// Handler serves the UI: index.html at the root and the assets beside it.
func Handler() http.Handler {
	return http.FileServer(http.FS(assets))
}
