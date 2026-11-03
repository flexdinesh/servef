package review

import (
	"slices"
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
	"github.com/flexdinesh/servef/pkg/contract"
)

// rows renders a layout's rows as "-old +new kind text" for comparison.
func rows(hunk Hunk) []string {
	out := make([]string, len(hunk.Lines))
	for i, line := range hunk.Lines {
		prefix := map[LineKind]string{Context: " ", Deleted: "-", Added: "+"}[line.Kind]
		out[i] = prefix + line.Text
	}
	return out
}

// The hunks, rows, and gaps are the server's line diff for the fixture's
// sides (testdata/api/default/diff-hunks.json, checked against the server).
func TestLayHunksFromTheServerLineDiff(t *testing.T) {
	layout := Lay(fakeservef.Diff(t, "diff-hunks.json"), false)
	if layout.Kind != Text || layout.Additions != 4 || layout.Deletions != 4 || layout.Language != "text" {
		t.Fatalf("layout: %+v", layout)
	}
	if len(layout.Hunks) != 3 || layout.Hidden != 10 {
		t.Fatalf("hunks %d, hidden %d", len(layout.Hunks), layout.Hidden)
	}
	first := layout.Hunks[0]
	if want := []string{" line 1", "-line 2", "+line 2 changed", " line 3", " line 4", " line 5", " line 6"}; !slices.Equal(rows(first), want) {
		t.Fatalf("first hunk %q, want %q", rows(first), want)
	}
	if first.Section() != "" || layout.Hunks[1].Section() != "9 unmodified lines" || layout.Hunks[2].Section() != "1 unmodified line" {
		t.Fatalf("sections %q %q %q", first.Section(), layout.Hunks[1].Section(), layout.Hunks[2].Section())
	}
	second := layout.Hunks[1]
	if second.Lines[4] != (Line{Kind: Deleted, Old: 20, Text: "line 20"}) || second.Lines[5] != (Line{Kind: Context, Old: 21, New: 20, Text: "line 21"}) {
		t.Fatalf("second hunk rows %+v", second.Lines[3:6])
	}
	// The last line lost its final newline; whitespace-only changes show.
	last := rows(layout.Hunks[2])
	if want := []string{"-line 30", "+  line 30"}; !slices.Equal(last[4:6], want) {
		t.Fatalf("last hunk %q", last)
	}
	if want := []string{"+inserted", " line 39", "-line 40", "+line 40"}; !slices.Equal(last[len(last)-4:], want) {
		t.Fatalf("last hunk end %q", last)
	}
	for _, hunk := range layout.Hunks {
		old, new := 0, 0
		for _, line := range hunk.Lines {
			if line.Kind != Added {
				old++
			}
			if line.Kind != Deleted {
				new++
			}
		}
		if old != hunk.OldCount || new != hunk.NewCount {
			t.Fatalf("hunk %+v has %d old and %d new rows", hunk, old, new)
		}
	}
}

// A full file joins the gaps into one hunk covering both sides.
func TestLayFullFile(t *testing.T) {
	diff := fakeservef.Diff(t, "diff-hunks.json")
	layout := Lay(diff, true)
	if len(layout.Hunks) != 1 || layout.Hidden != 0 {
		t.Fatalf("full: %d hunks, %d hidden", len(layout.Hunks), layout.Hidden)
	}
	hunk := layout.Hunks[0]
	if hunk.OldStart != 1 || hunk.NewStart != 1 || hunk.OldCount != 40 || hunk.NewCount != 40 {
		t.Fatalf("full hunk %+v", hunk)
	}
	old, new := layout.Sides()
	for i := range old {
		if old[i] != i+1 {
			t.Fatalf("old side line %d is file line %d", i, old[i])
		}
	}
	for i := range new {
		if new[i] != i+1 {
			t.Fatalf("new side line %d is file line %d", i, new[i])
		}
	}
	if hunk.Lines[7] != (Line{Kind: Context, Old: 7, New: 7, Text: "line 7"}) {
		t.Fatalf("gap row %+v", hunk.Lines[7])
	}
}

func TestSidesFollowRows(t *testing.T) {
	layout := Lay(fakeservef.Diff(t, "diff-modified.json"), false)
	old, new := layout.Sides()
	if want := []int{3, 4, 5, 6, 7, 8}; !slices.Equal(old, want) || !slices.Equal(new, want) {
		t.Fatalf("sides %v %v", old, new)
	}
	if layout.Language != "go" {
		t.Fatalf("language %q", layout.Language)
	}
}

func TestLayKinds(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		kind    Kind
		message string
	}{
		{"diff-binary.json", Binary, "Binary file changed · 12.0 KB → 14.0 KB"},
		{"diff-too-large.json", TooLarge, "Large file (6.3 MB): diff is off."},
		{"diff-renamed.json", Unchanged, "File contents are unchanged."},
		{"diff-added.json", Text, ""},
		{"diff-deleted.json", Text, ""},
		{"diff-untracked.json", Text, ""},
	} {
		layout := Lay(fakeservef.Diff(t, tc.fixture), false)
		if layout.Kind != tc.kind || layout.Message != tc.message {
			t.Errorf("%s: kind %d %q, want %d %q", tc.fixture, layout.Kind, layout.Message, tc.kind, tc.message)
		}
	}
	added := Lay(fakeservef.Diff(t, "diff-added.json"), false)
	if old, new := added.Sides(); len(old) != 0 || len(new) != 2 || added.Hunks[0].Lines[0].Kind != Added {
		t.Fatalf("added: %v %v %+v", old, new, added.Hunks)
	}
	deleted := Lay(fakeservef.Diff(t, "diff-deleted.json"), false)
	if old, new := deleted.Sides(); len(old) != 8 || len(new) != 0 {
		t.Fatalf("deleted: %v %v", old, new)
	}
}

func TestLayWithoutLineDiffOrHunks(t *testing.T) {
	diff := fakeservef.Diff(t, "diff-modified.json")
	diff.LineDiff = nil
	if layout := Lay(diff, false); layout.Kind != Unavailable || layout.Message != "Couldn't compute this diff." {
		t.Fatalf("no line diff: %+v", layout)
	}
	diff.LineDiff = &contract.LineDiff{IgnoreWhitespace: true}
	if layout := Lay(diff, false); layout.Kind != Unchanged || layout.Message != "No changes apart from whitespace." {
		t.Fatalf("whitespace only: %+v", layout)
	}
	added := contract.DiffSide{Size: 4, Binary: true}
	if got := binaryLabel(nil, &added); got != "Binary file added · 4 B" {
		t.Fatalf("added binary: %q", got)
	}
	if got := binaryLabel(&added, nil); got != "Binary file deleted · 4 B" {
		t.Fatalf("deleted binary: %q", got)
	}
}

// Rows never index past a side, even if a line diff disagrees with it.
func TestLayToleratesShortSides(t *testing.T) {
	text := "a\r\nb\r\n"
	diff := contract.DiffResponse{
		Status:   contract.ChangeModified,
		Original: &contract.DiffSide{Content: &text}, Modified: &contract.DiffSide{Content: &text},
		LineDiff: &contract.LineDiff{Hunks: []contract.DiffHunk{{OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3, Blocks: []contract.DiffBlock{{Kind: contract.DiffBlockContext, OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3}}}}},
	}
	if got := rows(Lay(diff, false).Hunks[0]); !slices.Equal(got, []string{" a", " b", " "}) {
		t.Fatalf("rows %q", got)
	}
}

func TestRows(t *testing.T) {
	if got := Rows([]string{"a", "b", "c"}, []int{2, 3, 4, 0}); !slices.Equal(got, []string{"b", "c", "", ""}) {
		t.Fatalf("rows %q", got)
	}
}

func TestLabels(t *testing.T) {
	renamed := contract.ChangedFile{Path: "b.md", OldPath: "a.md", Status: contract.ChangeRenamed}
	if Label(renamed) != "a.md → b.md" || Letter(renamed.Status) != "R" || StatusName(renamed.Status) != "Renamed" {
		t.Fatal("renamed labels")
	}
	for status, letter := range map[contract.ChangeStatus]string{contract.ChangeModified: "M", contract.ChangeAdded: "A", contract.ChangeUntracked: "U", contract.ChangeDeleted: "D"} {
		if Letter(status) != letter {
			t.Errorf("%s: %s", status, Letter(status))
		}
	}
	if ChangeCount(1) != "1 change" || ChangeCount(3) != "3 changes" {
		t.Fatal(ChangeCount(1), ChangeCount(3))
	}
	if Counts(3, 1) != "+3 −1" {
		t.Fatal(Counts(3, 1))
	}
}
