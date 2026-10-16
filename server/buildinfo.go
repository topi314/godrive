package server

import (
	"fmt"
	"runtime/debug"
	"time"
)

// BuildInfo is populated from Go's embedded build metadata (see runtime/debug).
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime time.Time
	Modified  bool
}

func ReadBuildInfo() BuildInfo {
	bi := BuildInfo{
		Version: "unknown",
		Commit:  "unknown",
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return bi
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		bi.Version = v
	} else {
		bi.Version = "devel"
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				bi.Commit = s.Value
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				bi.BuildTime = t.UTC()
			}
		case "vcs.modified":
			bi.Modified = s.Value == "true"
		}
	}
	return bi
}

func (b BuildInfo) Format() string {
	commit := b.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if b.Modified && commit != "unknown" {
		commit += "-dirty"
	}
	if b.BuildTime.IsZero() {
		return fmt.Sprintf("%s (%s)", b.Version, commit)
	}
	return fmt.Sprintf("%s (%s) built %s", b.Version, commit, b.BuildTime.Format(time.RFC3339))
}
