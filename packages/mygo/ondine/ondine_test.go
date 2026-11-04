package ondine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmbeddedModuleMatchesPin(t *testing.T) {
	sum := sha256.Sum256(Wasm())
	if got := hex.EncodeToString(sum[:]); got != WasmSHA256 {
		t.Fatalf("all.wasm sha256 %s, pinned %s: run packages/mygo/scripts/sync-wasm.sh", got, WasmSHA256)
	}
}

type golden struct {
	Options struct {
		IDPrefix string  `json:"idPrefix"`
		MaxWidth float64 `json:"maxWidth"`
		Padding  float64 `json:"padding"`
		Now      int64   `json:"now"`
	} `json:"options"`
	Renders map[string]struct {
		OK      bool    `json:"ok"`
		Type    string  `json:"type"`
		Width   float64 `json:"width"`
		Height  float64 `json:"height"`
		Links   []Link  `json:"links"`
		SVG     string  `json:"svg"`
		Code    string  `json:"code"`
		Line    int     `json:"line"`
		Column  int     `json:"column"`
		Snippet string  `json:"snippet"`
		Message string  `json:"message"`
	} `json:"renders"`
}

func readGolden(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "renders.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Renders) == 0 {
		t.Fatal("no goldens")
	}
	return g
}

// Every sample renders exactly as Render.create(...).render does in the
// web app's JavaScript (testdata/renders.json, from scripts/goldens.mjs):
// the same SVG, size, type, and links, or the same diagnostic.
func TestRenderMatchesJavaScript(t *testing.T) {
	g := readGolden(t)
	opts := Options{IDPrefix: g.Options.IDPrefix, MaxWidth: g.Options.MaxWidth, Padding: g.Options.Padding, Now: time.UnixMilli(g.Options.Now)}
	for file, want := range g.Renders {
		source, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		diagram, err := Render(string(source), opts)
		if !want.OK {
			var rejected *Error
			if !errors.As(err, &rejected) {
				t.Errorf("%s: error %v, want a render error", file, err)
				continue
			}
			wantErr := Error{Code: want.Code, Type: want.Type, Line: want.Line, Column: want.Column, Snippet: want.Snippet, Message: want.Message}
			if *rejected != wantErr {
				t.Errorf("%s: error %+v, want %+v", file, *rejected, wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		if diagram.Type != want.Type || diagram.Width != want.Width || diagram.Height != want.Height || len(diagram.Links) != len(want.Links) {
			t.Errorf("%s: %s %vx%v %d links, want %s %vx%v %d", file, diagram.Type, diagram.Width, diagram.Height, len(diagram.Links), want.Type, want.Width, want.Height, len(want.Links))
		}
		for i := range min(len(diagram.Links), len(want.Links)) {
			if diagram.Links[i] != want.Links[i] {
				t.Errorf("%s: link %d %+v, want %+v", file, i, diagram.Links[i], want.Links[i])
			}
		}
		if diagram.SVG != want.SVG {
			t.Errorf("%s: SVG differs from the web's:\n got  %.300s\n want %.300s", file, diagram.SVG, want.SVG)
		}
	}
}

// The web's diagram.test.ts cases: a syntax error keeps ondine's
// diagnostic with its line and caret snippet.
func TestSyntaxErrorDiagnostic(t *testing.T) {
	_, err := Render("flowchart LR\n    A --> B\n    B --> [oops\n", Options{})
	var rejected *Error
	if !errors.As(err, &rejected) {
		t.Fatalf("error %v", err)
	}
	want := "Parse error on line 3:\n    B --> [oops\n----------^\nExpecting 'NODE_STRING', got 'SQS'"
	if rejected.Code != "syntax" || rejected.Line != 3 || rejected.Column != 11 || rejected.Message != want || rejected.Type != "flowchart" {
		t.Fatalf("diagnostic %+v", *rejected)
	}
	if !strings.HasSuffix(rejected.Snippet, "^") || rejected.Error() != want {
		t.Fatalf("snippet %q", rejected.Snippet)
	}
	// The instance survives a rejected diagram.
	if d, err := Render("flowchart LR\n  A --> B", Options{}); err != nil || !strings.HasPrefix(d.SVG, "<svg") {
		t.Fatalf("after an error: %v", err)
	}
}

func TestLimits(t *testing.T) {
	_, err := Render("flowchart LR\n"+strings.Repeat("  A --> B\n", 200), Options{Limit: 100})
	var rejected *Error
	if !errors.As(err, &rejected) || rejected.Code != "limit" {
		t.Fatalf("limit: %v", err)
	}
}

func TestConcurrentRenders(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			d, err := Render("sequenceDiagram\n  A->>B: hi", Options{IDPrefix: strings.Repeat("p", i+1)})
			if err != nil || d.Type != "sequence" {
				t.Errorf("render %d: %v %q", i, err, d.Type)
			}
		})
	}
	wg.Wait()
}
