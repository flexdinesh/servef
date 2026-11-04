package highlights

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestEmbeddedModuleMatchesPin(t *testing.T) {
	sum := sha256.Sum256(Wasm())
	if got := hex.EncodeToString(sum[:]); got != WasmSHA256 {
		t.Fatalf("highlights.wasm sha256 %s, pinned %s: run packages/mygo/scripts/sync-wasm.sh", got, WasmSHA256)
	}
}

// pairs renders lines as the goldens store them: [content, scope] pairs.
func pairs(src string, lines []Line) [][][2]string {
	out := make([][][2]string, len(lines))
	for i, line := range lines {
		out[i] = [][2]string{}
		for _, token := range line.Tokens {
			out[i] = append(out[i], [2]string{src[token.Start:token.End], token.Scope.String()})
		}
	}
	return out
}

// Every sample tokenizes exactly as codeToTokens does in the web app's
// JavaScript (testdata/tokens.json, from scripts/goldens.mjs).
func TestTokensMatchJavaScript(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string][][][2]string
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden) == 0 {
		t.Fatal("no goldens")
	}
	for key, want := range golden {
		file, limit := key, 0
		if name, ok := strings.CutSuffix(key, "@1000"); ok {
			file, limit = name, 1000
		}
		src, err := os.ReadFile(filepath.Join("testdata", "samples", file))
		if err != nil {
			t.Fatal(err)
		}
		lang := file[strings.LastIndexByte(file, '.')+1:]
		lines, err := Tokens(lang, string(src), Options{MaxLineLength: limit})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if got := pairs(string(src), lines); !reflect.DeepEqual(got, want) {
			for i := range max(len(got), len(want)) {
				var g, w [][2]string
				if i < len(got) {
					g = got[i]
				}
				if i < len(want) {
					w = want[i]
				}
				if !reflect.DeepEqual(g, w) {
					t.Errorf("%s line %d:\n got  %q\n want %q", key, i, g, w)
					break
				}
			}
		}
	}
}

func TestLineStarts(t *testing.T) {
	src := "a\r\nbb\n\nc"
	lines, err := Tokens("js", src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var starts []int
	for _, line := range lines {
		starts = append(starts, line.Start)
	}
	if want := []int{0, 3, 6, 7}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("starts %v, want %v", starts, want)
	}
	if plain := Plain(src); !reflect.DeepEqual(pairs(src, plain), [][][2]string{{{"a", "none"}}, {{"bb", "none"}}, {}, {{"c", "none"}}}) {
		t.Fatalf("plain %q", pairs(src, plain))
	}
}

func TestFallbacks(t *testing.T) {
	src := "x := 1\ny"
	lines, err := Tokens("klingon", src, Options{})
	if !errors.Is(err, ErrUnknownLanguage) || !reflect.DeepEqual(lines, Plain(src)) {
		t.Fatalf("unknown language: %v %v", lines, err)
	}
	bad := "ok\n\xff\xfe"
	if lines, err := Tokens("go", bad, Options{}); err == nil || !reflect.DeepEqual(lines, Plain(bad)) {
		t.Fatalf("invalid UTF-8: %v %v", lines, err)
	}
	if !Supported("TypeScript") || !Supported("text") || Supported("klingon") {
		t.Fatal("Supported")
	}
	if Scope(0).String() != "none" || Scope(255).String() != "" || len(Scopes()) != len(scopeNames) {
		t.Fatal("scopes")
	}
}

// A source past the memory bound fails to plain lines; the next call
// still works.
func TestMemoryBound(t *testing.T) {
	huge := strings.Repeat("x", maxPages*pageSize)
	if _, err := Tokens("js", huge, Options{}); err == nil {
		t.Fatal("source over the memory bound tokenized")
	}
	if lines, err := Tokens("js", "let a", Options{}); err != nil || len(lines) != 1 || len(lines[0].Tokens) != 2 {
		t.Fatalf("after the bound: %v %v", lines, err)
	}
}

func TestConcurrentCalls(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "samples", "main.golang"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := Tokens("go", string(src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if got, err := Tokens("go", string(src), Options{}); err != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("concurrent tokens differ: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()
	poolMu.Lock()
	defer poolMu.Unlock()
	if len(idle) > poolSize {
		t.Fatalf("%d idle instances, want at most %d", len(idle), poolSize)
	}
}

func TestIDClasses(t *testing.T) {
	for r, want := range map[rune][2]bool{
		'a': {true, true}, '_': {false, true}, '1': {false, true}, '$': {false, false},
		'é': {true, true}, '中': {true, true}, '𝑥': {true, true}, '℮': {true, true},
		'\u0301': {false, true}, '\u200d': {false, true}, '😀': {false, false}, '·': {false, true},
	} {
		if got := [2]bool{isIDStart(r), isIDContinue(r)}; got != want {
			t.Errorf("%U: start/continue %v, want %v", r, got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	syntax := map[string]Setting{
		"comment":         {Color: "gray", FontStyle: "italic"},
		"keyword":         {Color: "red"},
		"emphasis":        {FontStyle: "italic"},
		"emphasis.strong": {FontWeight: 700},
		"string.special":  {FontWeight: 600},
		"string":          {Color: "green", FontStyle: "italic"},
	}
	for scope, want := range map[string]Setting{
		"comment.doc":     {Color: "gray", FontStyle: "italic"},
		"keyword.control": {Color: "red"},
		"emphasis.strong": {FontWeight: 700},
		// The nearest entry's font settings, the nearest color.
		"string.special.symbol": {Color: "green", FontWeight: 600},
	} {
		if got, ok := Resolve(syntax, scope); !ok || got != want {
			t.Errorf("%s: %+v, want %+v", scope, got, want)
		}
	}
	if _, ok := Resolve(syntax, "type"); ok {
		t.Error("type has no entry")
	}
}

// A panic on an instance closes it and frees its slot; concurrent calls
// past the slot count all finish and leave no slot taken.
func TestInstancesAreReleased(t *testing.T) {
	inst, err := acquire()
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = inst.use(func() ([]uint32, error) { panic("boom") })
	}()
	if !inst.module.IsClosed() || len(slots) != 0 {
		t.Fatalf("after a panic: closed %v, slots in use %d", inst.module.IsClosed(), len(slots))
	}
	var wg sync.WaitGroup
	for i := range 4 * cap(slots) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Tokens("go", fmt.Sprintf("var x = %d", i), Options{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	poolMu.Lock()
	defer poolMu.Unlock()
	if len(slots) != 0 || len(idle) > poolSize {
		t.Fatalf("slots in use %d, idle %d", len(slots), len(idle))
	}
}
