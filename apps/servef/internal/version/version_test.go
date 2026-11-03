package version

import (
	"runtime/debug"
	"testing"
)

func TestStringDefaultsToDevelopmentVersion(t *testing.T) {
	setBuildMetadata(t, "dev", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true)

	if got, want := String(), "servef dev"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestStringIncludesReleaseMetadata(t *testing.T) {
	setBuildMetadata(t, "0.1.0", "abc123", "2026-09-13T00:00:00Z", nil, false)

	if got, want := String(), "servef 0.1.0 abc123 2026-09-13T00:00:00Z"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestStringFallsBackToTaggedGoInstallMetadata(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.2.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "def456"},
			{Key: "vcs.time", Value: "2026-09-14T00:00:00Z"},
		},
	}
	setBuildMetadata(t, "dev", "", "", info, true)

	if got, want := String(), "servef 0.2.0 def456 2026-09-14T00:00:00Z"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestNumberOmitsBuildMetadata(t *testing.T) {
	setBuildMetadata(t, "0.1.0", "abc123", "2026-09-13T00:00:00Z", nil, false)

	if got, want := Number(), "0.1.0"; got != want {
		t.Fatalf("Number() = %q, want %q", got, want)
	}
}

func setBuildMetadata(t *testing.T, version, commit, date string, info *debug.BuildInfo, ok bool) {
	t.Helper()
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	oldReadBuildInfo := readBuildInfo
	Version = version
	Commit = commit
	Date = date
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, ok }
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
		readBuildInfo = oldReadBuildInfo
	})
}
