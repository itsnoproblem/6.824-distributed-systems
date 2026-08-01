// Package static embeds the app's client-side assets.
package static

import "embed"

//go:embed *.css *.js
var FS embed.FS
