// Package web embeds the built frontend (web/dist) into the binary. `make web`
// or `make web-docker` populates dist; without it the directory holds only
// .gitkeep and the server answers 404 for the app, which keeps `go build` and
// `go test` free of Node.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// fs.Sub fails only for an invalid path; "dist" is a constant.
		panic(err)
	}
	return sub
}
