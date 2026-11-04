// Package highlights runs Pierre's Highlights (@pierre/highlights, a
// WebAssembly syntax highlighter; Apache-2.0, LICENSE.md and NOTICE.md) in
// pure Go under wazero: the module the web app embeds, called the way its
// JavaScript glue calls it (dist/highlighter.js, tokens.js), so Tokens gives
// the lines and token scopes codeToTokens gives there. Go supplies the
// module's only imports, env.is_id_start and env.is_id_continue (Unicode
// XID).
//
// The module compiles once per process, on first use, through the shared
// wasmcache; instances are pooled, one per call in flight, and an instance
// whose memory grew past 8 MiB is dropped after its call.
package highlights

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/flexdinesh/servef/packages/mygo/wasmcache"
)

//go:embed highlights.wasm
var wasmBytes []byte

// Wasm returns the embedded module's bytes.
func Wasm() []byte { return wasmBytes }

// Scope is a token's scope (Highlights' token type), by ID.
type Scope uint8

// String is the scope's name ("keyword.control"), or "" for an unknown ID.
func (s Scope) String() string {
	if int(s) < len(scopeNames) {
		return scopeNames[s]
	}
	return ""
}

// Scopes lists every scope's name by ID; ID 0 is "none".
func Scopes() []string { return slices.Clone(scopeNames[:]) }

// Languages lists every name and alias Highlights knows, sorted;
// "plain", "plaintext", "text", and "txt" have no lexer.
func Languages() []string {
	names := make([]string, 0, len(languageIDs))
	for name := range languageIDs {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Supported reports whether lang (any case) is a name or alias Highlights
// knows, as isSupportedLanguage does; the plain names are supported.
func Supported(lang string) bool {
	_, ok := languageIDs[strings.ToLower(lang)]
	return ok
}

// Token is a run of source in one scope, from Start to End, byte offsets
// in the source.
type Token struct {
	Start, End int
	Scope      Scope
}

// Line is one line's tokens; Start is the line's byte offset in the
// source. Lines break after "\n"; a "\r" before it belongs to no token.
type Line struct {
	Start  int
	Tokens []Token
}

// Options are codeToTokens' options that change tokens.
type Options struct {
	// MaxLineLength, when positive, gives a line this many UTF-16 code
	// units or longer one token of scope 0, as tokenizeMaxLineLength does.
	MaxLineLength int
}

// ErrUnknownLanguage is Tokens' error for a language Highlights does not
// know.
var ErrUnknownLanguage = errors.New("highlights: unknown language")

// Tokens tokenizes src in lang. On any failure (an unknown language,
// invalid UTF-8, an engine error, or a panic) it returns Plain(src) and
// the error, so callers can always show the lines.
func Tokens(lang, src string, opts Options) (lines []Line, err error) {
	defer func() {
		if r := recover(); r != nil {
			lines, err = Plain(src), fmt.Errorf("highlights: panic: %v", r)
		}
	}()
	id, ok := languageIDs[strings.ToLower(lang)]
	if !ok {
		return Plain(src), fmt.Errorf("%w: %q", ErrUnknownLanguage, lang)
	}
	if !utf8.ValidString(src) {
		return Plain(src), errors.New("highlights: source is not valid UTF-8")
	}
	inst, err := acquire()
	if err != nil {
		return Plain(src), err
	}
	recs, err := inst.records(id, src)
	release(inst, err)
	if err != nil {
		return Plain(src), err
	}
	return lineTokens(src, recs, opts.MaxLineLength), nil
}

// Plain is src's lines with one scope-0 token each (none for empty lines),
// broken as Tokens breaks them.
func Plain(src string) []Line {
	lines := make([]Line, 0, strings.Count(src, "\n")+1)
	start := 0
	for {
		end := strings.IndexByte(src[start:], '\n')
		last := end < 0
		if last {
			end = len(src)
		} else {
			end += start
		}
		text := end
		if !last && text > start && src[text-1] == '\r' {
			text--
		}
		line := Line{Start: start}
		if text > start {
			line.Tokens = []Token{{Start: start, End: text}}
		}
		lines = append(lines, line)
		if last {
			return lines
		}
		start = end + 1
	}
}

// lineMarker ends a line's records; its offset is the next line's start.
const lineMarker = 0xffffffff

// lineTokens converts the module's UTF-16 line records to lines of byte
// offsets, as lineRecordsToTokens does: records are (end, scope) pairs, a
// line ends with (next line start, lineMarker), and empty runs drop.
func lineTokens(src string, recs []uint32, maxLineLength int) []Line {
	at := offsets{src: src}
	var lines []Line
	start := uint32(0)
	for rec := 0; ; rec += 2 {
		first := rec
		for rec < len(recs) && recs[rec+1] != lineMarker {
			rec += 2
		}
		end := start
		if rec != first {
			end = recs[rec-2]
		}
		line := Line{Start: at.byteAt(start)}
		if maxLineLength > 0 && int(end-start) >= maxLineLength {
			if end > start {
				line.Tokens = []Token{{Start: line.Start, End: at.byteAt(end)}}
			}
		} else {
			line.Tokens = make([]Token, 0, (rec-first)/2)
			for i := first; i < rec; i += 2 {
				if e := recs[i]; e > start {
					line.Tokens = append(line.Tokens, Token{Start: at.byteAt(start), End: at.byteAt(e), Scope: Scope(recs[i+1])})
					start = e
				}
			}
		}
		lines = append(lines, line)
		if rec >= len(recs) {
			return lines
		}
		start = recs[rec]
	}
}

// offsets maps increasing UTF-16 offsets in src to byte offsets.
type offsets struct {
	src         string
	byteOffset  int
	utf16Offset uint32
}

func (o *offsets) byteAt(utf16Offset uint32) int {
	if utf16Offset < o.utf16Offset {
		o.byteOffset, o.utf16Offset = 0, 0
	}
	for o.utf16Offset < utf16Offset && o.byteOffset < len(o.src) {
		r, size := utf8.DecodeRuneInString(o.src[o.byteOffset:])
		units := uint32(1)
		if r >= 0x10000 {
			units = 2
		}
		if o.utf16Offset+units > utf16Offset {
			break // inside a surrogate pair: keep the rune whole
		}
		o.byteOffset += size
		o.utf16Offset += units
	}
	return o.byteOffset
}

// The module's layout (highlighter.js): a header in page 0, the input
// from page 1, output where the header says.
const (
	pageSize = 65536
	// maxPages bounds an instance's memory (64 MiB).
	maxPages = 1024
	// keepPages is the most memory an instance returns to the pool with.
	keepPages = 128
	// poolSize is the most idle instances kept.
	poolSize = 2
)

type engine struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

var (
	engineOnce sync.Once
	shared     *engine
	engineErr  error

	poolMu sync.Mutex
	idle   []*instance
)

func loadEngine() (*engine, error) {
	engineOnce.Do(func() {
		ctx := context.Background()
		rt := wasmcache.NewRuntime(maxPages)
		_, err := rt.NewHostModuleBuilder("env").
			NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, ptr, n uint32) uint32 {
			return idClass(m, ptr, n, isIDStart)
		}).Export("is_id_start").
			NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, ptr, n uint32) uint32 {
			return idClass(m, ptr, n, isIDContinue)
		}).Export("is_id_continue").
			Instantiate(ctx)
		if err != nil {
			engineErr = fmt.Errorf("highlights: host module: %w", err)
			return
		}
		compiled, err := wasmcache.Compile(rt, wasmBytes)
		if err != nil {
			engineErr = fmt.Errorf("highlights: compile: %w", err)
			return
		}
		shared = &engine{runtime: rt, compiled: compiled}
	})
	return shared, engineErr
}

// Warm compiles the module ahead of the first Tokens call.
func Warm() error {
	_, err := loadEngine()
	return err
}

// idClass decodes the UTF-8 code point at ptr (n bytes) and reports
// whether it is in the class, as the glue's #idClass does.
func idClass(m api.Module, ptr, n uint32, in func(rune) bool) uint32 {
	b, ok := m.Memory().Read(ptr, n)
	if !ok {
		return 0
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size <= 1 {
		return 0
	}
	if in(r) {
		return 1
	}
	return 0
}

type instance struct {
	module api.Module
}

func acquire() (*instance, error) {
	poolMu.Lock()
	if n := len(idle); n > 0 {
		inst := idle[n-1]
		idle = idle[:n-1]
		poolMu.Unlock()
		return inst, nil
	}
	poolMu.Unlock()
	e, err := loadEngine()
	if err != nil {
		return nil, err
	}
	module, err := e.runtime.InstantiateModule(context.Background(), e.compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, fmt.Errorf("highlights: instantiate: %w", err)
	}
	return &instance{module: module}, nil
}

// release pools a healthy instance with little memory and closes the rest.
func release(inst *instance, err error) {
	if err == nil && inst.module.Memory().Size()/pageSize <= keepPages {
		poolMu.Lock()
		if len(idle) < poolSize {
			idle = append(idle, inst)
			poolMu.Unlock()
			return
		}
		poolMu.Unlock()
	}
	_ = inst.module.Close(context.Background())
}

// records lexes src in language id with output mode 3 (UTF-16 line
// records) and returns a copy of the records.
func (inst *instance) records(id uint8, src string) ([]uint32, error) {
	mem := inst.module.Memory()
	n := uint32(len(src))
	if need := 1 + (uint64(n)+96+pageSize-1)/pageSize; need > uint64(mem.Size()/pageSize) {
		if _, ok := mem.Grow(uint32(need - uint64(mem.Size()/pageSize))); !ok {
			return nil, errors.New("highlights: source too large")
		}
	}
	if !mem.WriteString(pageSize, src) || !mem.WriteByte(pageSize+n, 0) {
		return nil, errors.New("highlights: write source")
	}
	var header [6]byte
	header[0], header[1] = id, 3
	binary.LittleEndian.PutUint32(header[2:], n)
	if !mem.Write(0, header[:]) {
		return nil, errors.New("highlights: write header")
	}
	if _, err := inst.module.ExportedFunction("highlight").Call(context.Background()); err != nil {
		return nil, fmt.Errorf("highlights: %w", err)
	}
	out, ok := mem.Read(6, 8)
	if !ok {
		return nil, errors.New("highlights: read output header")
	}
	start, length := binary.LittleEndian.Uint32(out), binary.LittleEndian.Uint32(out[4:])
	raw, ok := mem.Read(start, length&^3)
	if !ok {
		return nil, errors.New("highlights: read output")
	}
	recs := make([]uint32, len(raw)/4)
	for i := range recs {
		recs[i] = binary.LittleEndian.Uint32(raw[4*i:])
	}
	if len(recs)%2 != 0 {
		return nil, errors.New("highlights: odd record count")
	}
	return recs, nil
}

// SameLexer reports whether two names or aliases select one lexer, as
// "go" and "golang" do.
func SameLexer(a, b string) bool {
	x, ok1 := languageIDs[strings.ToLower(a)]
	y, ok2 := languageIDs[strings.ToLower(b)]
	return ok1 && ok2 && x == y
}
