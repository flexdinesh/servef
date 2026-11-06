package walkthrough

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func stored(project, id string, updated time.Time) Walkthrough {
	return Walkthrough{ID: id, Project: project, Title: "Title " + id, Steps: []Step{{Title: "one"}, {Title: "two"}}, CreatedAt: updated, UpdatedAt: updated}
}

func TestStorePutsGetsListsAndDeletes(t *testing.T) {
	store := NewStore(t.TempDir())
	start := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	created, err := store.Put(stored("/a", "first", start))
	if err != nil || !created {
		t.Fatalf("Put first = %v, %v", created, err)
	}
	if _, err := store.Put(stored("/a", "second", start.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(stored("/b", "first", start)); err != nil {
		t.Fatal(err)
	}
	replaced := stored("/a", "first", start.Add(2*time.Minute))
	replaced.Title = "Replaced"
	if created, err := store.Put(replaced); err != nil || created {
		t.Fatalf("Put replacement = %v, %v", created, err)
	}

	list, err := store.List("/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "first" || list[0].Title != "Replaced" || list[0].StepCount != 2 || list[1].ID != "second" {
		t.Fatalf("List = %+v", list)
	}
	got, err := store.Get("/a", "first")
	if err != nil || got.Title != "Replaced" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := store.Get("/a", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing error = %v", err)
	}
	if _, err := store.Get("/a", "../b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get unsafe ID error = %v", err)
	}
	if err := store.Delete("/a", "first"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("/a", "first"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v", err)
	}
	if list, _ := store.List("/a"); len(list) != 1 || list[0].ID != "second" {
		t.Errorf("List after delete = %+v", list)
	}
	if list, _ := store.List("/b"); len(list) != 1 {
		t.Errorf("other project = %+v", list)
	}
	if list, err := store.List("/never"); err != nil || len(list) != 0 {
		t.Errorf("empty project = %+v, %v", list, err)
	}
}

func TestStoreWritesPrivateFilesAtomically(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	directory := filepath.Join(t.TempDir(), "walkthroughs")
	store := NewStore(directory)
	if _, err := store.Put(stored("/a", "first", time.Now())); err != nil {
		t.Fatal(err)
	}
	projectDirectory := store.projectDirectory("/a")
	info, err := os.Stat(projectDirectory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("project directory mode = %v, %v", info, err)
	}
	entries, err := os.ReadDir(projectDirectory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "first.json" {
		t.Fatalf("entries = %v, %v (no temporary files remain)", entries, err)
	}
	info, err = os.Stat(filepath.Join(projectDirectory, "first.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, %v", info, err)
	}
}

func TestStoreEvictsLeastRecentlyUpdatedPastTheLimit(t *testing.T) {
	store := NewStore(t.TempDir())
	store.limit = 3
	start := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	for i := range 3 {
		if _, err := store.Put(stored("/a", fmt.Sprintf("w%d", i), start.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	// Updating the oldest makes it the newest, so w1 is evicted next.
	if _, err := store.Put(stored("/a", "w0", start.Add(10*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(stored("/a", "w3", start.Add(11*time.Minute))); err != nil {
		t.Fatal(err)
	}
	list, err := store.List("/a")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, summary := range list {
		ids = append(ids, summary.ID)
	}
	if fmt.Sprint(ids) != "[w3 w0 w2]" {
		t.Fatalf("ids = %v, want [w3 w0 w2]", ids)
	}
}

func TestStoreSkipsForeignAndCorruptFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Put(stored("/a", "good", time.Now())); err != nil {
		t.Fatal(err)
	}
	directory := store.projectDirectory("/a")
	foreign, _ := os.ReadFile(filepath.Join(directory, "good.json"))
	if err := os.WriteFile(filepath.Join(directory, "renamed.json"), foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "corrupt.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := store.List("/a")
	if err != nil || len(list) != 1 || list[0].ID != "good" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if _, err := store.Get("/a", "renamed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get renamed error = %v", err)
	}
}
