package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCatalogCoversCodeFamiliesWithInstallGuidance(t *testing.T) {
	want := []string{"go", "javascript", "jsx", "typescript", "tsx", "python", "rust", "c", "cpp", "java", "kotlin", "ruby", "php", "lua", "swift", "css", "scss"}
	for _, machine := range []host{{os: "linux", distro: "ubuntu debian", tools: map[string]bool{"apt-get": true, "brew": true}}, {os: "darwin", tools: map[string]bool{"brew": true}}, {os: "windows", tools: map[string]bool{"scoop": true}}, {os: "linux", distro: "unknown", tools: map[string]bool{}}} {
		t.Run(machine.os+machine.distro, func(t *testing.T) {
			families := map[string]bool{}
			for _, server := range builtinServers(machine) {
				for family := range server.Families {
					if families[family] {
						t.Fatalf("duplicate family %s", family)
					}
					families[family] = true
				}
				for _, install := range server.Install {
					if install.Docs == "" || install.Label == "" || len(install.Prerequisites) == 0 {
						t.Fatalf("incomplete install guidance: %+v", install)
					}
					if machine.os != "darwin" && strings.Contains(install.Command, "brew ") {
						t.Fatalf("brew recipe on %s: %+v", machine.os, install)
					}
				}
			}
			for _, family := range want {
				if !families[family] {
					t.Errorf("missing %s", family)
				}
			}
		})
	}
}

func TestLinuxPackageRecipeRequiresMatchingDistroAndManager(t *testing.T) {
	for _, tt := range []struct {
		distro string
		tools  map[string]bool
		want   string
	}{
		{"ubuntu debian", map[string]bool{"apt-get": true}, "sudo apt-get install clangd"},
		{"fedora", map[string]bool{"dnf": true}, "sudo dnf install clang-tools-extra"},
		{"arch", map[string]bool{"pacman": true}, "sudo pacman -S clang"},
		{"unknown", map[string]bool{"apt-get": true}, ""},
		{"ubuntu", map[string]bool{"dnf": true}, ""},
	} {
		t.Run(tt.distro+tt.want, func(t *testing.T) {
			for _, server := range builtinServers(host{os: "linux", distro: tt.distro, tools: tt.tools}) {
				if server.Name == "clangd" && server.Install[0].Command != tt.want {
					t.Fatalf("recipe=%q want %q", server.Install[0].Command, tt.want)
				}
			}
		})
	}
}

func TestResolveNativeTypeScriptBeforeAdapter(t *testing.T) {
	root := t.TempDir()
	server := Server{Name: "typescript-language-server", Args: []string{"--stdio"}, NativeTypeScript: true}
	adapter := writeTool(t, filepath.Join(root, "node_modules", ".bin", "typescript-language-server"))
	project := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	none := func(name string) (string, bool) { return "node.exe", runtime.GOOS == "windows" && name == "node" }
	resolved, ok := server.Resolve(project, none)
	if !ok || resolved.Path != adapter {
		t.Fatalf("adapter = %+v,%v", resolved, ok)
	}
	legacy := writeTypeScript(t, root, "typescript", "6.0.0", "tsc")
	resolved, ok = server.Resolve(project, none)
	if !ok || resolved.Path != adapter {
		t.Fatalf("classic tsc mistaken for LSP: %+v,%v", resolved, ok)
	}
	if err := os.RemoveAll(filepath.Dir(filepath.Dir(legacy))); err != nil {
		t.Fatal(err)
	}
	native := writeTypeScript(t, root, "typescript", "7.0.2", "tsc")
	resolved, ok = server.Resolve(project, none)
	wantPath, wantArgs := native, []string{"--lsp", "--stdio"}
	if runtime.GOOS == "windows" {
		wantPath = "node.exe"
		wantArgs = append([]string{native}, wantArgs...)
	}
	if !ok || resolved.Path != wantPath || resolved.Name != "tsc" || !reflect.DeepEqual(resolved.Args, wantArgs) {
		t.Fatalf("native = %+v,%v", resolved, ok)
	}
}

func TestResolveNativePreviewAndGlobalMetadata(t *testing.T) {
	root := t.TempDir()
	native := writeTypeScript(t, root, "@typescript/native-preview", "7.0.0-dev", "tsgo")
	server := Server{Name: "typescript-language-server", NativeTypeScript: true}
	lookup := func(name string) (string, bool) {
		if runtime.GOOS == "windows" && name == "node" {
			return "node.exe", true
		}
		return native, name == "tsgo"
	}
	resolved, ok := server.Resolve(t.TempDir(), lookup)
	wantPath := native
	if runtime.GOOS == "windows" {
		wantPath = "node.exe"
	}
	if !ok || resolved.Path != wantPath || resolved.Name != "tsgo" {
		t.Fatalf("native preview = %+v,%v", resolved, ok)
	}
}

func TestResolveUsesLocalToolsAndDoesNotExecuteThem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execute-bit semantics")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "ran")
	path := filepath.Join(root, "node_modules", ".bin", "pyright-langserver")
	writeTool(t, path)
	if err := os.WriteFile(path, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := Server{Name: "pyright-langserver", Args: []string{"--stdio"}}
	resolved, ok := server.Resolve(root, func(string) (string, bool) { t.Fatal("looked globally before project"); return "", false })
	if !ok || resolved.Path != path {
		t.Fatalf("local tool = %+v,%v", resolved, ok)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("discovery executed tool")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.Resolve(root, func(string) (string, bool) { return "", false }); ok {
		t.Fatal("accepted nonexecutable tool")
	}
}

func writeTool(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTypeScript(t *testing.T, root, name, version, bin string) string {
	t.Helper()
	dir := filepath.Join(root, "node_modules", filepath.FromSlash(name))
	executable := writeTool(t, filepath.Join(dir, "bin", bin))
	pkg := struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Bin     map[string]string `json:"bin"`
	}{name, version, map[string]string{bin: "bin/" + bin}}
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return executable
}

func TestResolveNearestAdapterPrecedesAncestorAndGlobalNativeTypeScript(t *testing.T) {
	for _, location := range []string{"global", "ancestor"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "packages", "app")
			adapter := writeTool(t, filepath.Join(project, "node_modules", ".bin", "typescript-language-server"))
			nativeRoot := t.TempDir()
			if location == "ancestor" {
				nativeRoot = root
			}
			native := writeTypeScript(t, nativeRoot, "typescript", "7.0.2", "tsc")
			marker := filepath.Join(nativeRoot, "executed")
			if err := os.WriteFile(native, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			server := Server{Name: "typescript-language-server", Args: []string{"--stdio"}, NativeTypeScript: true}
			resolved, ok := server.Resolve(project, func(name string) (string, bool) {
				t.Fatalf("looked up %s globally before project adapter", name)
				return native, name == "tsc"
			})
			if !ok || resolved.Path != adapter || resolved.Name != "typescript-language-server" || !reflect.DeepEqual(resolved.Args, []string{"--stdio"}) {
				t.Fatalf("local adapter = %+v,%v", resolved, ok)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("discovery executed native TypeScript")
			}
		})
	}
}
