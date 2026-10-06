package acl

import (
	"path"
	"strings"
)

// NormalizePath returns an absolute path without trailing slash (except root "/").
func NormalizePath(p string) string {
	if p == "" {
		return "/"
	}
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "." {
		return "/"
	}
	return p
}

// AncestorPaths returns [path, parent, ..., "/"] from leaf to root.
func AncestorPaths(filePath string) []string {
	filePath = NormalizePath(filePath)
	var paths []string
	for {
		paths = append(paths, filePath)
		if filePath == "/" {
			break
		}
		parent := path.Dir(filePath)
		if parent == filePath {
			break
		}
		filePath = parent
	}
	return paths
}

// AncestorPathsRootFirst returns ["/", ..., parent, path].
func AncestorPathsRootFirst(filePath string) []string {
	paths := AncestorPaths(filePath)
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	return paths
}

func LikeUnder(prefix string) string {
	prefix = NormalizePath(prefix)
	if prefix == "/" {
		return "/%"
	}
	return prefix + "/%"
}

func IsSelfOrUnder(p, root string) bool {
	p, root = NormalizePath(p), NormalizePath(root)
	if p == root {
		return true
	}
	if root == "/" {
		return p != "/"
	}
	return strings.HasPrefix(p, root+"/")
}

// RemapPrefix rewrites from → to, and from/… → to/….
func RemapPrefix(p, from, to string) string {
	p, from, to = NormalizePath(p), NormalizePath(from), NormalizePath(to)
	if p == from {
		return to
	}
	if from == "/" {
		return NormalizePath(to + p)
	}
	if strings.HasPrefix(p, from+"/") {
		return to + p[len(from):]
	}
	return p
}
