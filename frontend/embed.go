//go:build !dev

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distEmbed embed.FS

func Dist() (fs.FS, error) {
	return fs.Sub(distEmbed, "dist")
}
