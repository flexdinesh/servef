//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommittedBatchSnapshotPreservesBytesPathsAndModes(t *testing.T) {
	root := repository(t)
	contents := map[string]string{
		" leading and trailing ": "",
		"tab\tand\nnewline":      "\x00binary\nheader blob 123\n\xff",
		"unicode-日":              strings.Repeat("large\x00blob\n", 20000),
		"executable":             "#!/bin/sh\nprintf original\\n\n",
	}
	// More requests and output than typical pipe buffers. A writer that sends
	// every request before reading responses would deadlock this capture.
	for i := range 1700 {
		contents[fmt.Sprintf("many/%04d", i)] = fmt.Sprintf("blob %d\n%s", i, strings.Repeat("x", 100))
	}
	for name, content := range contents {
		put(t, root, name, content)
	}
	if err := os.Chmod(filepath.Join(root, "executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tab\tand\nnewline", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	commit(t, root, "batch fixture")
	s := snap(t, root)
	if len(s.Entries) != len(contents)+1 {
		t.Fatalf("captured %d entries, want %d", len(s.Entries), len(contents)+1)
	}
	working, err := readWorkingSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Entries, working.Entries) {
		t.Fatal("batch and working snapshots disagree on paths, modes, or content")
	}
	out := t.TempDir()
	if err := materialize(out, s.Entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range s.Entries {
		if e.Blob != contentIdentity(e.Content) {
			t.Fatalf("wrong content identity for %q", e.Path)
		}
	}
	for name, want := range contents {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(got) != want {
			t.Fatalf("batch changed %q: %v", name, err)
		}
	}
	info, err := os.Stat(filepath.Join(out, "executable"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("batch lost executable mode: %v", err)
	}
	link, err := os.Readlink(filepath.Join(out, "link"))
	if err != nil || link != "tab\tand\nnewline" {
		t.Fatalf("batch changed symlink: %q %v", link, err)
	}
}

func TestCommittedBatchSnapshotRejectsEscapingSymlinkOnMaterialization(t *testing.T) {
	root := repository(t)
	if err := os.Symlink("../outside", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	commit(t, root, "escape")
	if err := materialize(t.TempDir(), snap(t, root).Entries); err == nil {
		t.Fatal("escaping committed symlink accepted")
	}
}

func TestWorkingSnapshotCapturesEditsNewFilesAndDeletion(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/edited.ts", "before")
	put(t, root, "apps/web/deleted.ts", "remove")
	put(t, root, ".gitignore", "ignored\n")
	commit(t, root, "initial")
	put(t, root, "apps/web/edited.ts", "after")
	put(t, root, " new file.ts", "new")
	put(t, root, "ignored", "excluded")
	put(t, root, "apps/web/.env.local", "personal")
	put(t, root, "node_modules/poison", "excluded")
	if err := os.Remove(filepath.Join(root, "apps/web/deleted.ts")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, " new file.ts"), 0755); err != nil {
		t.Fatal(err)
	}
	s, err := readWorkingSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "apps/web/edited.ts", "later")
	out := t.TempDir()
	if err := materialize(out, s.Entries); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"apps/web/edited.ts": "after", " new file.ts": "new"} {
		content, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(content) != want {
			t.Fatalf("snapshot %s = %q, error %v", name, content, err)
		}
	}
	for _, name := range []string{"apps/web/deleted.ts", "ignored", "apps/web/.env.local", "node_modules/poison"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected snapshot path %s: %v", name, err)
		}
	}
	info, err := os.Stat(filepath.Join(out, " new file.ts"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("executable mode lost: %v", err)
	}
}

func TestWorkingSnapshotRejectsEscapingSymlinks(t *testing.T) {
	root := repository(t)
	put(t, root, "initial", "initial")
	commit(t, root, "initial")
	if err := os.Symlink("../outside", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := readWorkingSnapshot(root); err == nil {
		t.Fatal("escaping working-copy link accepted")
	}
}

func TestWorkingAndCommittedContentsHaveSameIdentity(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/source.ts", "source")
	commit(t, root, "initial")
	committed := snap(t, root)
	working, err := readWorkingSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range chosen(options{Suite: "full"}) {
		if key(t, committed, group) != key(t, working, group) {
			t.Fatalf("identical contents have different %s input identity", group)
		}
	}
}
