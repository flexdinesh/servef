// Package wasmcache compiles the WebAssembly ports in packages/mygo
// (highlights, ondine) on runtimes that share one wazero compilation cache
// per process: in memory unless SetDir names a directory, where compiled
// code persists across runs and later launches skip compiling.
package wasmcache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/tetratelabs/wazero"
)

var (
	mu    sync.Mutex
	dir   string
	cache wazero.CompilationCache
)

// ErrInUse is SetDir's error once a module has compiled.
var ErrInUse = errors.New("wasmcache: the cache is already in use")

// SetDir keeps compiled modules in dir (created if missing), which the
// cache owns: Load clears it when it holds an entry wazero rejects. Call it
// before the first module compiles; later calls return ErrInUse.
func SetDir(d string) error {
	mu.Lock()
	defer mu.Unlock()
	if cache != nil {
		return ErrInUse
	}
	c, err := wazero.NewCompilationCacheWithDir(d)
	if err != nil {
		return err
	}
	dir, cache = d, c
	return nil
}

// shared returns the process's cache and its directory ("" for memory).
func shared() (wazero.CompilationCache, string) {
	mu.Lock()
	defer mu.Unlock()
	if cache == nil {
		cache = wazero.NewCompilationCache()
	}
	return cache, dir
}

// reset replaces a disk cache that failed with a fresh one in the same
// directory, after removing what it held; memory only when that fails.
func reset(failed wazero.CompilationCache) {
	mu.Lock()
	defer mu.Unlock()
	if cache != failed || dir == "" {
		return // another Load reset it already
	}
	if err := os.RemoveAll(dir); err == nil {
		if c, err := wazero.NewCompilationCacheWithDir(dir); err == nil {
			cache = c
			return
		}
	}
	dir, cache = "", wazero.NewCompilationCache()
}

// newRuntime returns a runtime on c whose memories grow to at most
// limitPages pages of 64 KiB, and whose calls stop when their context is
// done.
func newRuntime(limitPages uint32, c wazero.CompilationCache) wazero.Runtime {
	config := wazero.NewRuntimeConfig().WithCompilationCache(c).WithMemoryLimitPages(limitPages).WithCloseOnContextDone(true)
	return wazero.NewRuntimeWithConfig(context.Background(), config)
}

// Load compiles wasm (or loads it from the cache directory) on a new
// runtime and returns both. A disk cache wazero cannot use (a corrupt
// entry fails with "compilationcache: checksum mismatch") is cleared and
// the module compiled again, then compiled in memory alone. Compiling a
// large module allocates tens of MiB the Go runtime would otherwise keep
// (Ondine: about 40 MiB of RSS); Load returns them to the system.
func Load(limitPages uint32, wasm []byte) (wazero.Runtime, wazero.CompiledModule, error) {
	defer debug.FreeOSMemory()
	c, d := shared()
	rt, compiled, err := compile(limitPages, c, wasm)
	if err == nil || d == "" {
		return rt, compiled, err
	}
	reset(c)
	c, _ = shared()
	if rt, compiled, err = compile(limitPages, c, wasm); err == nil {
		return rt, compiled, nil
	}
	return compile(limitPages, wazero.NewCompilationCache(), wasm)
}

func compile(limitPages uint32, c wazero.CompilationCache, wasm []byte) (wazero.Runtime, wazero.CompiledModule, error) {
	ctx := context.Background()
	rt := newRuntime(limitPages, c)
	compiled, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, nil, fmt.Errorf("compile: %w", err)
	}
	return rt, compiled, nil
}
