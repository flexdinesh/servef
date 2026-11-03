//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	for _, group := range groups {
		if key(t, committed, group) != key(t, working, group) {
			t.Fatalf("identical contents have different %s input identity", group)
		}
	}
}
