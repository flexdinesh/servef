package main

import (
	"math"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestSelectTagStartsSeriesAtPatchZero(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	if got, want := selectedTag(t, series, nil, nil), "v0.1.0"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagIncrementsHighestPatch(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	tags := []string{"v0.1.1", "v0.1.9", "v0.1.3", "v0.2.20"}
	if got, want := selectedTag(t, series, tags, nil), "v0.1.10"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagIgnoresMalformedAndPrereleaseTags(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	tags := []string{"v0.1.2", "v0.1.10-beta.1", "v0.1.04", "v0.1.next", "0.1.20"}
	if got, want := selectedTag(t, series, tags, nil), "v0.1.3"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagReusesReleaseAtHead(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	tags := []string{"v0.1.0", "v0.1.1"}
	if got, want := selectedTag(t, series, tags, []string{"v0.1.1"}), "v0.1.1"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagStartsChangedSeriesAtPatchZero(t *testing.T) {
	series := mustParseSeries(t, "1.0")
	tags := []string{"v0.1.8", "v0.2.4"}
	if got, want := selectedTag(t, series, tags, nil), "v1.0.0"; got != want {
		t.Fatalf("selectTag() = %q, want %q", got, want)
	}
}

func TestSelectTagRejectsPatchOverflow(t *testing.T) {
	series := mustParseSeries(t, "0.1")
	_, err := selectTag(series, []string{"v0.1." + strconv.Itoa(math.MaxInt)}, nil)
	if err == nil {
		t.Fatal("selectTag() succeeded")
	}
}

func TestParseSeriesRejectsFullOrPaddedVersions(t *testing.T) {
	for _, value := range []string{"0.1.0", "v0.1", "01.2", "1.02", "next"} {
		if _, err := parseSeries(value); err == nil {
			t.Fatalf("parseSeries(%q) succeeded", value)
		}
	}
}

func TestTagsBuiltOnHeadFindsReleaseCommitsOnTopOfHead(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "--quiet")
	git(t, dir, "commit", "--allow-empty", "--quiet", "-m", "older main")
	git(t, dir, "tag", "v0.1.0")
	git(t, dir, "commit", "--allow-empty", "--quiet", "-m", "main")
	main := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "tag", "v0.1.1")
	git(t, dir, "commit", "--allow-empty", "--quiet", "-m", "release: v0.1.2")
	git(t, dir, "tag", "v0.1.2")
	git(t, dir, "checkout", "--quiet", "--detach", main)

	got, err := tagsBuiltOnHead(dir, []string{"v0.1.0", "v0.1.1", "v0.1.2", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v0.1.2"}; !slices.Equal(got, want) {
		t.Fatalf("tagsBuiltOnHead() = %q, want %q", got, want)
	}
}

func git(t *testing.T, dir string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = dir
	command.Env = append(command.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func mustParseSeries(t *testing.T, value string) releaseSeries {
	t.Helper()
	series, err := parseSeries(value)
	if err != nil {
		t.Fatal(err)
	}
	return series
}

func selectedTag(t *testing.T, series releaseSeries, tags, headTags []string) string {
	t.Helper()
	tag, err := selectTag(series, tags, headTags)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}
