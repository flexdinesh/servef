package docview

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

var (
	textType     = &contract.FileType{ID: "text", Label: "Plain text", Language: "text", DefaultMode: contract.ModeSource}
	goType       = &contract.FileType{ID: "go", Language: "go", DefaultMode: contract.ModeSource}
	markdownType = &contract.FileType{ID: "markdown", Language: "markdown", PreviewKind: contract.PreviewMarkdown, PreviewLabel: "Preview", DefaultMode: contract.ModeSource}
	jsonType     = &contract.FileType{ID: "json", Label: "JSON", Language: "json", Formattable: true, DefaultMode: contract.ModeSource}
	svgType      = &contract.FileType{ID: "svg", Language: "xml", PreviewKind: contract.PreviewImage, PreviewLabel: "Preview", DefaultMode: contract.ModePreview}
	rasterType   = &contract.FileType{ID: "image", PreviewKind: contract.PreviewImage, PreviewLabel: "Preview", DefaultMode: contract.ModePreview}
)

const snapshotHash = "3577ec6a1b2c3d4e5f60718293a4b5c6d7e8f901"

func page(kind *contract.FileType) contract.PageData {
	return contract.PageData{Project: "/p", Selected: "dir/file", HasFile: true, DocumentType: kind, FileSize: 6_500_000}
}

func with(p contract.PageData, change func(*contract.PageData)) contract.PageData {
	change(&p)
	return p
}

func binary(p *contract.PageData)   { p.Binary = true }
func tooLarge(p *contract.PageData) { p.TooLarge, p.PreviewDisabled = true, true }
func large(p *contract.PageData)    { p.PreviewDisabled = true }
func snapshot(p *contract.PageData) { p.At = snapshotHash }

// document-view.test.ts: declared capabilities select toolbar modes.
func TestModes(t *testing.T) {
	for _, c := range []struct {
		page contract.PageData
		want []Mode
	}{
		{page(markdownType), []Mode{Source, Preview}},
		{page(jsonType), []Mode{Source, Format}},
		{page(svgType), []Mode{Preview, Source}},
		{with(page(rasterType), binary), []Mode{Preview}},
		{page(textType), []Mode{Source}},
		{with(page(textType), binary), nil},
		{with(page(markdownType), binary), nil},
		{contract.PageData{}, nil},
		{with(page(markdownType), tooLarge), nil},
		{with(page(jsonType), tooLarge), nil},
	} {
		if got := Modes(c.page); !slices.Equal(got, c.want) {
			t.Errorf("Modes(%+v) = %v, want %v", c.page.DocumentType, got, c.want)
		}
	}
}

// Snapshots offer Source and Format, never Preview.
func TestSnapshotModes(t *testing.T) {
	for _, c := range []struct {
		kind *contract.FileType
		want []Mode
	}{{markdownType, []Mode{Source}}, {jsonType, []Mode{Source, Format}}, {svgType, []Mode{Source}}} {
		if got := Modes(with(page(c.kind), snapshot)); !slices.Equal(got, c.want) {
			t.Errorf("snapshot %s: %v", c.kind.ID, got)
		}
	}
	if got := Modes(with(with(page(rasterType), snapshot), binary)); got != nil {
		t.Errorf("raster snapshot: %v", got)
	}
	if got := Resolve(with(page(markdownType), snapshot), Preview); got != Source {
		t.Errorf("snapshot preview resolves to %q", got)
	}
	if got := Resolve(with(page(svgType), snapshot), ""); got != Source {
		t.Errorf("snapshot svg resolves to %q", got)
	}
}

// Modes resolve from request, then default, and respect large-file limits.
func TestResolve(t *testing.T) {
	for _, c := range []struct {
		page      contract.PageData
		requested Mode
		want      Mode
	}{
		{page(markdownType), "", Source},
		{page(markdownType), Preview, Preview},
		{page(svgType), "", Preview},
		{page(jsonType), "", Source},
		{page(jsonType), Format, Format},
		{page(jsonType), Preview, Source},
		{with(page(markdownType), large), Preview, Source},
		{with(page(jsonType), large), Format, Source},
		{with(page(textType), binary), "", ""},
		{with(page(markdownType), binary), Preview, ""},
		{with(page(jsonType), tooLarge), Source, ""},
	} {
		if got := Resolve(c.page, c.requested); got != c.want {
			t.Errorf("Resolve(%s, %q) = %q, want %q", c.page.DocumentType.ID, c.requested, got, c.want)
		}
	}
	if got := DisabledReason(with(page(jsonType), large), Format); got != "Large file (6.2 MB): formatting is off" {
		t.Errorf("format reason %q", got)
	}
	if got := DisabledReason(with(page(markdownType), large), Preview); got != "Large file (6.2 MB): preview is off" {
		t.Errorf("preview reason %q", got)
	}
	if got := DisabledReason(with(page(jsonType), large), Source); got != "" {
		t.Errorf("source reason %q", got)
	}
	if got := DisabledReason(page(markdownType), Preview); got != "" {
		t.Errorf("no reason %q", got)
	}
}

func TestRequested(t *testing.T) {
	for _, c := range []struct {
		chosen Mode
		lines  bool
		prefer bool
		want   Mode
	}{
		{"", false, false, ""},
		{"", false, true, Preview},
		// A line anchor opens Source unless the document chose a view.
		{"", true, true, Source},
		{Format, true, true, Format},
		{Preview, false, false, Preview},
	} {
		if got := Requested(c.chosen, c.lines, c.prefer); got != c.want {
			t.Errorf("Requested(%+v) = %q", c, got)
		}
	}
}

// Format results replace Source only once ready.
func TestDecideFormat(t *testing.T) {
	json := page(jsonType)
	for _, format := range []*FormatState{nil, {Kind: FormatLoading}} {
		if v := Decide(json, Format, format); v.Kind != Code || !v.FormatPending || len(v.Notices) != 0 {
			t.Fatalf("pending: %+v", v)
		}
	}
	if v := Decide(json, Format, &FormatState{Kind: FormatReady, Text: "{}\n"}); v.Kind != Formatted || v.Text != "{}\n" || v.FormatPending {
		t.Fatalf("ready: %+v", v)
	}
	failed := Decide(json, Source, &FormatState{Kind: FormatFailed, Message: "bad"})
	if failed.Kind != Code || failed.FormatPending || !slices.Equal(failed.Notices, []Notice{{Danger, "Couldn't format: bad."}}) {
		t.Fatalf("failed: %+v", failed)
	}
	if v := Decide(page(svgType), Preview, nil); v.Kind != Image {
		t.Fatalf("image: %+v", v)
	}
	if v := Decide(page(markdownType), Preview, nil); v.Kind != Markdown {
		t.Fatalf("preview: %+v", v)
	}
	broken := with(page(markdownType), func(p *contract.PageData) { p.PreviewError = "boom" })
	if v := Decide(broken, Preview, nil); v.Kind != PreviewFailed || v.Message != "Could not render this preview: boom" {
		t.Fatalf("preview error: %+v", v)
	}
	if v := Decide(contract.PageData{}, "", nil); v.Kind != None {
		t.Fatalf("empty: %+v", v)
	}
}

func TestDecideNotes(t *testing.T) {
	b := Decide(with(page(textType), binary), "", nil)
	if b.Kind != Binary || b.Note != (Note{"Binary file", "file · 6.2 MB", "servef shows text and images only."}) {
		t.Fatalf("binary: %+v", b)
	}
	for _, kind := range []*contract.FileType{markdownType, jsonType, textType} {
		v := Decide(with(page(kind), tooLarge), "", nil)
		if v.Kind != TooLarge || v.Note != (Note{"Too large to display", "file · 6.2 MB", "servef shows text files up to 32 MB."}) || v.Modes != nil {
			t.Fatalf("too large: %+v", v)
		}
	}

	plain := with(page(goType), func(p *contract.PageData) { p.HighlightDisabled = true })
	if v := Decide(plain, Source, nil); v.Highlight {
		t.Fatal("highlight disabled is honored")
	}

	notice := Decide(with(page(textType), large), Source, nil)
	if !slices.Equal(notice.Notices, []Notice{{Warning, "Large file (6.2 MB): preview and formatting are off."}}) {
		t.Fatalf("large notice: %+v", notice.Notices)
	}
	excluded := Decide(with(with(page(textType), large), func(p *contract.PageData) { p.SearchExcluded = true }), Source, nil)
	if !slices.Equal(excluded.Notices, []Notice{{Warning, "Large file (6.2 MB): preview and formatting are off. Not included in search."}}) {
		t.Fatalf("excluded notice: %+v", excluded.Notices)
	}
	snap := Decide(with(page(jsonType), snapshot), Source, nil)
	if !slices.Equal(snap.Notices, []Notice{{Neutral, "Snapshot at 3577ec6: read-only, without preview or code navigation."}}) {
		t.Fatalf("snapshot notice: %+v", snap.Notices)
	}
}

func TestDecideWithoutDocument(t *testing.T) {
	if v := Decide(contract.PageData{Project: "/p"}, "", nil); v.Kind != None {
		t.Fatalf("no selection: %+v", v)
	}
	failed := contract.PageData{Project: "/p", Selected: "gone.md", Error: "File not found"}
	if v := Decide(failed, "", nil); v.Kind != Failed || v.Message != "File not found" {
		t.Fatalf("failed: %+v", v)
	}
	unavailable := contract.PageData{ProjectUnavailable: true, Error: "Project unavailable"}
	if v := Decide(unavailable, "", nil); v.Kind != Failed {
		t.Fatalf("unavailable project: %+v", v)
	}
}

func TestModeLabel(t *testing.T) {
	image := &contract.FileType{PreviewLabel: "Image"}
	if got := ModeLabel(page(image), Preview); got != "Image" {
		t.Errorf("preview label %q", got)
	}
	if got := ModeLabel(page(textType), Preview); got != "Preview" {
		t.Errorf("default label %q", got)
	}
	if ModeLabel(page(jsonType), Format) != "Format" || ModeLabel(page(jsonType), Source) != "Source" {
		t.Error("format and source labels")
	}
}

func TestBytes(t *testing.T) {
	for size, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1536: "1.5 KB", 3 << 20: "3.0 MB", 2 << 30: "2.0 GB"} {
		if got := Bytes(size); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", size, got, want)
		}
	}
}

func TestEmptyCountsFiles(t *testing.T) {
	for count, want := range map[int]string{0: "0 files. Choose one in the tree.", 1: "1 file. Choose it in the tree.", 3: "3 files. Choose one in the tree."} {
		if got := Empty(count); got != want {
			t.Errorf("Empty(%d) = %q, want %q", count, got, want)
		}
	}
}
