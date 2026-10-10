package acl

import "strings"

// Reserved top-level path prefixes and a few root static asset names.
// Keep this list minimal so users can name files freely.
var reservedExact = map[string]struct{}{
	"/favicon.ico":       {},
	"/favicon.png":       {},
	"/favicon-light.png": {},
	"/robots.txt":        {},
}

var reservedPrefixes = []string{
	"/api",
	"/s",
	"/_nuxt",
	"/.uploads", // resumable-upload staging (local + S3); not a user folder
}

// IsReservedPath reports whether p conflicts with app routes, embedded static
// assets, or internal storage staging paths.
func IsReservedPath(p string) bool {
	p = NormalizePath(p)
	for _, prefix := range reservedPrefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	_, ok := reservedExact[p]
	return ok
}
