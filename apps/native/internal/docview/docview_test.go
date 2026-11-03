package docview

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

var (
	goType       = &contract.FileType{ID: "go", Language: "go", DefaultMode: contract.ModeSource}
	markdownType = &contract.FileType{ID: "markdown", Language: "markdown", PreviewKind: contract.PreviewMarkdown, PreviewLabel: "Preview", DefaultMode: contract.ModePreview}
	imageType    = &contract.FileType{ID: "png", PreviewKind: contract.PreviewImage, PreviewLabel: "Image", DefaultMode: contract.ModePreview}
)

func page(kind *contract.FileType) contract.PageData {
	return contract.PageData{Project: "/p", Selected: "f", HasFile: true, DocumentType: kind}
}

func TestDecide(t *testing.T) {
	code := Decide(page(goType), "")
	if code.Kind != Code || code.Mode != Source || !slices.Equal(code.Modes, []Mode{Source}) || code.Language != "go" || !code.Highlight {
		t.Fatalf("code: %+v", code)
	}

	md := Decide(page(markdownType), "")
	if md.Kind != Markdown || md.Mode != Preview || !slices.Equal(md.Modes, []Mode{Preview, Source}) {
		t.Fatalf("markdown default: %+v", md)
	}
	if v := Decide(page(markdownType), Source); v.Kind != Code || v.Mode != Source {
		t.Fatalf("markdown as source: %+v", v)
	}

	img := page(imageType)
	img.Binary = true
	if v := Decide(img, ""); v.Kind != Image || !slices.Equal(v.Modes, []Mode{Preview}) {
		t.Fatalf("binary image: %+v", v)
	}

	binary := page(goType)
	binary.Binary = true
	if v := Decide(binary, ""); v.Kind != Binary || v.Modes != nil {
		t.Fatalf("binary: %+v", v)
	}

	large := page(markdownType)
	large.PreviewDisabled, large.FileSize = true, 5*1024*1024
	if v := Decide(large, Preview); v.Kind != Code || v.Message != "Large file (5.0 MB): preview is off" {
		t.Fatalf("preview disabled: %+v", v)
	}

	huge := page(goType)
	huge.TooLarge, huge.FileSize = true, 40*1024*1024
	if v := Decide(huge, ""); v.Kind != TooLarge || v.Modes != nil {
		t.Fatalf("too large: %+v", v)
	}

	plain := page(goType)
	plain.HighlightDisabled = true
	if v := Decide(plain, ""); v.Highlight {
		t.Fatal("highlight disabled is honored")
	}

	snapshot := page(markdownType)
	snapshot.At = "0123456789012345678901234567890123456789"
	if v := Decide(snapshot, Preview); v.Kind != Code || !slices.Equal(v.Modes, []Mode{Source}) {
		t.Fatalf("snapshot has source only: %+v", v)
	}
}

func TestDecideWithoutDocument(t *testing.T) {
	if v := Decide(contract.PageData{Project: "/p"}, ""); v.Kind != None {
		t.Fatalf("no selection: %+v", v)
	}
	failed := contract.PageData{Project: "/p", Selected: "gone.md", Error: "File not found"}
	if v := Decide(failed, ""); v.Kind != Failed || v.Message != "File not found" {
		t.Fatalf("failed: %+v", v)
	}
	unavailable := contract.PageData{ProjectUnavailable: true, Error: "Project unavailable"}
	if v := Decide(unavailable, ""); v.Kind != Failed {
		t.Fatalf("unavailable project: %+v", v)
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
