// Package wasmcache holds the wazero compilation cache the WebAssembly ports
// in packages/mygo (highlights, ondine) share: one per process, in memory
// unless SetDir names a directory, where compiled code persists across runs
// and later launches skip compiling.
package wasmcache

import (
	"context"
	"errors"
	"runtime/debug"
	"sync"

	"github.com/tetratelabs/wazero"
)

var (
	mu    sync.Mutex
	cache wazero.CompilationCache
)

// ErrInUse is SetDir's error once a module has compiled.
var ErrInUse = errors.New("wasmcache: the cache is already in use")

// SetDir keeps compiled modules in dir (created if missing). Call it before
// the first module compiles; later calls return ErrInUse.
func SetDir(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	if cache != nil {
		return ErrInUse
	}
	c, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return err
	}
	cache = c
	return nil
}

// Cache returns the process's compilation cache.
func Cache() wazero.CompilationCache {
	mu.Lock()
	defer mu.Unlock()
	if cache == nil {
		cache = wazero.NewCompilationCache()
	}
	return cache
}

// NewRuntime returns a runtime on the shared cache whose memories grow to
// at most limitPages pages of 64 KiB.
func NewRuntime(limitPages uint32) wazero.Runtime {
	config := wazero.NewRuntimeConfig().WithCompilationCache(Cache()).WithMemoryLimitPages(limitPages)
	return wazero.NewRuntimeWithConfig(context.Background(), config)
}

// Compile compiles a module on rt (or loads it from the cache directory)
// and returns the garbage it made to the system at once: compiling a large
// module allocates tens of MiB the Go runtime would otherwise keep (Ondine:
// about 40 MiB of RSS). Modules compile once per process, so this runs
// once each.
func Compile(rt wazero.Runtime, wasm []byte) (wazero.CompiledModule, error) {
	compiled, err := rt.CompileModule(context.Background(), wasm)
	debug.FreeOSMemory()
	return compiled, err
}
