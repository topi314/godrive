package server

import (
	"errors"
	"path"
	"strings"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
)

// resolveShareUploadDir maps a client dir (browse /s/{id}/…, empty, or storage path)
// onto the share storage directory used as the relative-name base.
func resolveShareUploadDir(share database.Share, dir string) string {
	root := acl.NormalizePath(share.Path)
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "/" {
		return root
	}
	prefix := "/s/" + share.ID
	if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
		rest := strings.TrimPrefix(dir, prefix)
		rest = strings.TrimPrefix(rest, "/")
		target, err := joinShareTarget(root, rest)
		if err != nil {
			return root
		}
		return target
	}
	n := acl.NormalizePath(dir)
	if acl.IsSelfOrUnder(n, root) {
		return n
	}
	// Relative to share root (e.g. "inbox").
	target, err := joinShareTarget(root, strings.TrimPrefix(dir, "/"))
	if err != nil {
		return root
	}
	return target
}

// resolveUploadTarget maps a relative or absolute upload name onto a final path.
// Relative names join under baseDir; absolute names are used as-is after normalize.
func resolveUploadTarget(baseDir, name string) (target string, err error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return "", errors.New("missing name")
	}
	if strings.HasSuffix(name, "/") {
		return "", errors.New("name must be a file path")
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "." || seg == ".." {
			return "", errors.New("invalid path segment")
		}
	}
	var raw string
	if strings.HasPrefix(name, "/") {
		raw = name
	} else {
		raw = path.Join(acl.NormalizePath(baseDir), name)
	}
	target = acl.NormalizePath(raw)
	if target == "/" {
		return "", errors.New("invalid name")
	}
	for _, seg := range strings.Split(strings.Trim(target, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errors.New("invalid path segment")
		}
	}
	if !validEntryName(path.Base(target)) {
		return "", errors.New("invalid name")
	}
	if acl.IsReservedPath(target) {
		return "", errors.New("reserved path")
	}
	return target, nil
}
