// Package config embeds the curated YAML files of this directory into the
// binary (go-conventions, Layout). Each file's reader validates it: the news
// feeds in internal/news.
package config

import "embed"

// FS holds the curated files, by their names in this directory.
//
//go:embed *.yaml
var FS embed.FS
