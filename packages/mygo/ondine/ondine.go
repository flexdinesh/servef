// Package ondine renders Mermaid to SVG with Ondine (ondine, a Rust
// WebAssembly compiler for every diagram type; MIT, LICENSE) in pure Go
// under wazero: the all-diagram module the web app loads, called the way
// its JavaScript glue calls it (src/internal/Wasm.ts, src/Render.ts), so
// Render returns the SVG and diagnostics render returns there.
//
// The module has no imports. It compiles once per process, on the first
// Render (or Warm), through the shared wasmcache; a failed compile is
// tried again on the next Render. One instance serves the calls in turn;
// it is closed after a call that failed, panicked, ran past renderTimeout,
// or grew its memory past 32 MiB.
package ondine

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/flexdinesh/servef/packages/mygo/wasmcache"
)

//go:embed all.wasm
var wasmBytes []byte

// Wasm returns the embedded module's bytes.
func Wasm() []byte { return wasmBytes }

// Options are render's options; zero values are Ondine's defaults.
type Options struct {
	// AutoDirection lets a diagram that runs across run down when it does
	// not fit MaxWidth (default true).
	AutoDirection *bool `json:"autoDirection,omitempty"`
	// DirectionBreakpoint is the width under which AutoDirection turns a
	// diagram (default 640).
	DirectionBreakpoint float64 `json:"directionBreakpoint,omitempty"`
	// FontSize is in pixels (default 16).
	FontSize float64 `json:"fontSize,omitempty"`
	// IDPrefix keeps the element IDs of several diagrams apart.
	IDPrefix string `json:"idPrefix,omitempty"`
	// Limit is the most characters of source accepted (default 50000).
	Limit int `json:"limit,omitempty"`
	// MaxEdges is the most edges a flowchart may have (default 5000).
	MaxEdges int `json:"maxEdges,omitempty"`
	// MaxWidth is the width available to the diagram, in pixels.
	MaxWidth float64 `json:"maxWidth,omitempty"`
	// OutputLimit is the most characters of SVG produced (default 4000000).
	OutputLimit int `json:"outputLimit,omitempty"`
	// Padding is around the diagram, in pixels (default 8).
	Padding float64 `json:"padding,omitempty"`
	// Responsive scales the SVG to its container (default true).
	Responsive *bool `json:"responsive,omitempty"`
	// Now is the time gantt charts schedule against (default: now).
	Now time.Time `json:"-"`
}

// Link is a link drawn in a diagram.
type Link struct {
	Href     string
	ID       string
	Internal bool
}

// Diagram is a rendered diagram.
type Diagram struct {
	// SVG is the SVG document.
	SVG string
	// Type is the detected diagram type ("flowchart").
	Type string
	// Width and Height are in pixels.
	Width, Height float64
	// Links are in drawing order.
	Links []Link
}

// Error is a diagram Ondine could not render; its Message matches
// Mermaid's diagnostics (RenderError).
type Error struct {
	// Code is "limit", "semantic", "syntax", or "unsupported-diagram".
	Code string
	// Type is the detected diagram type, or "".
	Type string
	// Line and Column (one-based, UTF-16) locate the error, or are 0.
	Line, Column int
	// Snippet is the offending line with a caret under the column, or "".
	Snippet string
	Message string
}

func (e *Error) Error() string { return e.Message }

const (
	// maxPages bounds the instance's memory (128 MiB).
	maxPages = 2048
	// keepPages is the most memory the instance is kept with (32 MiB).
	keepPages = 512
	pageSize  = 65536
)

// renderTimeout bounds one Render; past it the instance is closed and
// Render fails (callers show the source).
var renderTimeout = 10 * time.Second

var (
	// mu guards the engine and the instance; a failed load is not kept, so
	// the next Render (Reload diagrams) tries again.
	mu       sync.Mutex
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	current  api.Module
)

func loadEngine() error {
	if compiled != nil {
		return nil
	}
	rt, c, err := wasmcache.Load(maxPages, wasmBytes)
	if err != nil {
		return fmt.Errorf("ondine: %w", err)
	}
	runtime, compiled = rt, c
	return nil
}

// Warm compiles the module ahead of the first Render.
func Warm() error {
	mu.Lock()
	defer mu.Unlock()
	return loadEngine()
}

// Render renders Mermaid source. A diagram Ondine rejects is an *Error;
// other errors are the engine's.
func Render(source string, opts Options) (diagram Diagram, err error) {
	defer func() {
		if r := recover(); r != nil {
			diagram, err = Diagram{}, fmt.Errorf("ondine: panic: %v", r)
		}
	}()
	options, err := serialize(opts)
	if err != nil {
		return Diagram{}, err
	}
	now := float64(time.Now().UnixMilli())
	if !opts.Now.IsZero() {
		now = float64(opts.Now.UnixMilli())
	}
	mu.Lock()
	defer mu.Unlock()
	// A panic (recovered above, after this) leaves the instance closed.
	healthy := false
	defer func() {
		if !healthy && current != nil {
			_ = current.Close(context.Background())
			current = nil
		}
	}()
	if err := loadEngine(); err != nil {
		healthy = true
		return Diagram{}, err
	}
	if current == nil {
		current, err = runtime.InstantiateModule(context.Background(), compiled, wazero.NewModuleConfig().WithName(""))
		if err != nil {
			current = nil
			return Diagram{}, fmt.Errorf("ondine: instantiate: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	diagram, err = render(ctx, current, source, options, now)
	var rejected *Error
	healthy = (err == nil || errors.As(err, &rejected)) && current.Memory().Size()/pageSize <= keepPages
	return diagram, err
}

// serialize is the glue's: the options as JSON without the time, or ""
// for none.
func serialize(opts Options) (string, error) {
	data, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	if string(data) == "{}" {
		return "", nil
	}
	return string(data), nil
}

func call(ctx context.Context, m api.Module, name string, params ...uint64) ([]uint64, error) {
	fn := m.ExportedFunction(name)
	if fn == nil {
		return nil, fmt.Errorf("ondine: no export %s", name)
	}
	results, err := fn.Call(ctx, params...)
	if err != nil {
		return nil, fmt.Errorf("ondine: %s: %w", name, err)
	}
	return results, nil
}

// render writes source and options into one allocation, calls render, and
// decodes the result, as Wasm.ts's instantiate does.
func render(ctx context.Context, m api.Module, source, options string, now float64) (Diagram, error) {
	if _, err := call(ctx, m, "reset"); err != nil {
		return Diagram{}, err
	}
	size := len(source) + len(options)
	if size > math.MaxInt32 {
		return Diagram{}, errors.New("ondine: source too large")
	}
	results, err := call(ctx, m, "alloc", uint64(size))
	if err != nil {
		return Diagram{}, err
	}
	at := uint32(results[0])
	mem := m.Memory()
	if !mem.WriteString(at, source) || !mem.WriteString(at+uint32(len(source)), options) {
		return Diagram{}, errors.New("ondine: write source")
	}
	results, err = call(ctx, m, "render", uint64(at), uint64(len(source)), uint64(at)+uint64(len(source)), uint64(len(options)), api.EncodeF64(now))
	if err != nil {
		return Diagram{}, err
	}
	r := reader{mem: mem, at: uint32(results[0])}
	diagram, err := r.decode()
	if err != nil {
		return Diagram{}, err
	}
	return diagram, r.err
}

// reader decodes a result: a status byte, then fields that are each a
// little-endian u32 length and its bytes.
type reader struct {
	mem api.Memory
	at  uint32
	err error
}

func (r *reader) field() []byte {
	if r.err != nil {
		return nil
	}
	n, ok := r.mem.ReadUint32Le(r.at)
	if !ok {
		r.err = errors.New("ondine: result out of range")
		return nil
	}
	b, ok := r.mem.Read(r.at+4, n)
	if !ok {
		r.err = errors.New("ondine: result out of range")
		return nil
	}
	r.at += 4 + n
	return b
}

func (r *reader) text() string { return string(r.field()) }

func (r *reader) uint32() uint32 {
	if b := r.field(); len(b) >= 4 {
		return binary.LittleEndian.Uint32(b)
	}
	if r.err == nil {
		r.err = errors.New("ondine: short field")
	}
	return 0
}

func (r *reader) float64() float64 {
	if b := r.field(); len(b) >= 8 {
		return math.Float64frombits(binary.LittleEndian.Uint64(b))
	}
	if r.err == nil {
		r.err = errors.New("ondine: short field")
	}
	return 0
}

func (r *reader) decode() (Diagram, error) {
	status, ok := r.mem.ReadByte(r.at)
	if !ok {
		return Diagram{}, errors.New("ondine: result out of range")
	}
	r.at++
	if status == 1 {
		e := &Error{Code: r.text(), Type: r.text()}
		e.Line = int(r.uint32())
		e.Column = int(r.uint32())
		e.Snippet = r.text()
		e.Message = r.text()
		if r.err != nil {
			return Diagram{}, r.err
		}
		return Diagram{}, e
	}
	d := Diagram{Type: r.text()}
	d.Width = r.float64()
	d.Height = r.float64()
	count := r.uint32()
	for i := uint32(0); i < count && r.err == nil; i++ {
		link := Link{Href: r.text(), ID: r.text()}
		b := r.field()
		link.Internal = len(b) > 0 && b[0] == 1
		d.Links = append(d.Links, link)
	}
	d.SVG = r.text()
	return d, r.err
}
