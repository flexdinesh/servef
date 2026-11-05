//go:build linux || darwin

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostToolObservationRejectsChangedExecutables(t *testing.T) {
	for _, change := range []string{"edit", "replace", "retarget", "mode"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "original", "original")
			put(t, root, "other", "other")
			path := filepath.Join(root, "tool")
			if err := os.Symlink("original", path); err != nil {
				t.Fatal(err)
			}
			tools := hostTools{Paths: map[string]string{"tool": path}, Files: map[string]hostToolFile{}}
			if err := tools.observe("tool", path); err != nil {
				t.Fatal(err)
			}
			if err := tools.unchanged(); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "edit":
				put(t, root, "original", "edited executable")
			case "replace":
				if err := os.Rename(filepath.Join(root, "other"), filepath.Join(root, "original")); err != nil {
					t.Fatal(err)
				}
			case "retarget":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("other", path); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(filepath.Join(root, "original"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := tools.unchanged(); err == nil {
				t.Fatal("changed toolchain accepted")
			}
		})
	}
}

func TestHostManifestsReuseInvocationProfile(t *testing.T) {
	s := snapshot{Entries: []entry{{Path: "mise.lock", Content: []byte("[[tools.go]]\nversion = \"1.26.10\"\n")}}}
	profile := &hostProfile{Identity: "observed tools"}
	for _, group := range []string{"repo", "web-static", "tooling-static", "site-static"} {
		m, first, err := manifest(s, group, "", nil, profile)
		if err != nil || m.HostTools != profile.Identity || m.host != profile {
			t.Fatalf("%s did not reuse profile: %v", group, err)
		}
		raw, err := json.Marshal(m)
		if err != nil || strings.Contains(string(raw), "\"host\"") || strings.Contains(string(raw), "\"Tools\"") {
			t.Fatalf("execution profile leaked into manifest: %s %v", raw, err)
		}
		_, second, err := manifest(s, group, "", nil, &hostProfile{Identity: "changed tools"})
		if err != nil || first == second {
			t.Fatalf("%s ignored changed tool identity: %v", group, err)
		}
		if _, _, err := manifest(s, group, "", nil, nil); err == nil {
			t.Fatal("locked host manifest accepted missing profile")
		}
	}
}

func TestPrivateDependencyCopiesDoNotShareWritableFiles(t *testing.T) {
	source := t.TempDir()
	put(t, source, "package/index.js", "original")
	if err := os.Symlink("package/index.js", filepath.Join(source, "entry.js")); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(t.TempDir(), "deps"), filepath.Join(t.TempDir(), "deps")
	for _, destination := range []string{first, second} {
		if err := copyPrivateTree(source, destination); err != nil {
			t.Fatal(err)
		}
	}
	put(t, first, "package/index.js", "changed")
	for _, directory := range []string{source, second} {
		content, err := os.ReadFile(filepath.Join(directory, "entry.js"))
		if err != nil || string(content) != "original" {
			t.Fatalf("private write crossed dependency boundary: %q %v", content, err)
		}
	}
}

func TestDependencyCopiesRejectSharedAbsoluteTargets(t *testing.T) {
	source := t.TempDir()
	put(t, source, "package.js", "original")
	if err := os.Symlink(filepath.Join(source, "package.js"), filepath.Join(source, "absolute.js")); err != nil {
		t.Fatal(err)
	}
	if err := copyPrivateTree(source, filepath.Join(t.TempDir(), "deps")); err == nil {
		t.Fatal("dependency copy retained absolute shared state")
	}
}

func TestReadOnlyPreparedDirectoriesProduceWritablePrivateCopies(t *testing.T) {
	source := t.TempDir()
	put(t, source, "module/file.go", "original")
	if err := os.Chmod(filepath.Join(source, "module"), 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(source, "module"), 0755) })
	destination := filepath.Join(t.TempDir(), "modules")
	if err := copyPrivateTree(source, destination); err != nil {
		t.Fatal(err)
	}
	put(t, destination, "module/extra.go", "private")
	if _, err := os.Stat(filepath.Join(source, "module/extra.go")); !os.IsNotExist(err) {
		t.Fatalf("new file crossed preparation boundary: %v", err)
	}
}

func TestHostEnvironmentSelectsPrivateStateAndPinnedPaths(t *testing.T) {
	t.Setenv("SERVEF_API_TARGET", "http://localhost:1234")
	t.Setenv("NODE_OPTIONS", "--require=/personal/config.js")
	t.Setenv("GIT_DIR", "/personal/repository")
	t.Setenv("PATH", "/personal/bin")
	tools := hostTools{Paths: map[string]string{"go": "/pinned/go/bin/go", "node": "/pinned/node/bin/node", "pnpm": "/pinned/pnpm/pnpm", "mise": "/pinned/mise", "git": "/usr/bin/git"}}
	env := hostEnvironment("/private/run", "/private/source", tools)
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "/personal") || strings.Contains(joined, "localhost:1234") {
		t.Fatalf("personal configuration inherited: %s", joined)
	}
	for _, expected := range []string{"HOME=/private/run/home", "TMPDIR=/private/run/tmp", "GOCACHE=/private/run/go-build", "GOMODCACHE=/private/run/go-mod", "MISE_DATA_DIR=/private/run/mise-data", "MISE_TRUSTED_CONFIG_PATHS=/private/source"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("private state missing %s", expected)
		}
	}
}

func TestLockedHostVersionsIgnorePlatformSections(t *testing.T) {
	entries := []entry{{Path: "mise.lock", Content: []byte("[[tools.go]]\nversion = \"1.26.10\"\n[tools.go.\"platforms.linux-x64\"]\nchecksum = \"sha256:abc\"\n[[tools.node]]\nversion = \"26.12.1\"\n[[tools.pnpm]]\nversion = \"11.24.0\"\n")}}
	versions := lockedHostVersions(entries)
	for name, want := range map[string]string{"go": "1.26.10", "node": "26.12.1", "pnpm": "11.24.0"} {
		if versions[name] != want {
			t.Fatalf("%s version = %s", name, versions[name])
		}
	}
}
