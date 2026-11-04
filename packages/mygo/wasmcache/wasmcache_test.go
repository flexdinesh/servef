package wasmcache_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"

	"github.com/flexdinesh/servef/packages/mygo/highlights"
	"github.com/flexdinesh/servef/packages/mygo/wasmcache"
)

// helperDir, when set, makes this test binary a child that only fills the
// cache in that directory.
const helperDir = "WASMCACHE_TEST_FILL"

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperDir); dir != "" {
		if err := wasmcache.SetDir(dir); err != nil {
			panic(err)
		}
		rt, _, err := wasmcache.Load(1024, highlights.Wasm())
		if err != nil {
			panic(err)
		}
		_ = rt.Close(context.Background())
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A corrupt entry in the cache directory (wazero: "compilationcache:
// checksum mismatch") does not disable the module: Load clears the
// directory and compiles again.
func TestCorruptDiskCacheRecovers(t *testing.T) {
	dir := t.TempDir()
	fill := exec.Command(os.Args[0], "-test.run=^$")
	fill.Env = append(os.Environ(), helperDir+"="+dir)
	if out, err := fill.CombinedOutput(); err != nil {
		t.Fatalf("filling the cache: %v\n%s", err, out)
	}
	var entries []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			entries = append(entries, path)
		}
		return nil
	})
	if len(entries) == 0 {
		t.Fatal("the cache is empty")
	}
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i := len(data) / 2; i < len(data); i += 97 {
			data[i] ^= 0xff
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// wazero alone, configured as Load configures it, fails on it.
	c, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	plain := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfig().WithCompilationCache(c).WithMemoryLimitPages(1024).WithCloseOnContextDone(true))
	_, err = plain.CompileModule(context.Background(), highlights.Wasm())
	_ = plain.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("wazero on the corrupt cache: %v", err)
	}

	if err := wasmcache.SetDir(dir); err != nil {
		t.Fatal(err)
	}
	rt, compiled, err := wasmcache.Load(1024, highlights.Wasm())
	if err != nil {
		t.Fatalf("Load on the corrupt cache: %v", err)
	}
	defer rt.Close(context.Background())
	if compiled == nil {
		t.Fatal("no module")
	}
	// The directory holds a good entry again.
	again := exec.Command(os.Args[0], "-test.run=^$")
	again.Env = append(os.Environ(), helperDir+"="+dir)
	if out, err := again.CombinedOutput(); err != nil {
		t.Fatalf("loading from the rewritten cache: %v\n%s", err, out)
	}
}
