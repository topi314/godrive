//go:build dev

package frontend

import (
	"errors"
	"io/fs"
)

// Dist is unavailable in -tags dev builds (use the Nuxt dev server instead).
func Dist() (fs.FS, error) {
	return nil, errors.New("frontend not embedded in dev builds")
}
