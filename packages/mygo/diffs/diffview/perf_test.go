package diffview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/diffs"
)

// queue runs the viewer's background work when the test says so.
type queue struct{ work []func() func() }

func (q *queue) run(work func() func()) { q.work = append(q.work, work) }

func (q *queue) drain() {
	for len(q.work) > 0 {
		work := q.work[0]
		q.work = q.work[1:]
		if apply := work(); apply != nil {
			apply()
		}
	}
}

// changedPairs is n changed pairs, each a word apart.
func changedPairs(n int) *diffs.Diff {
	var old, new strings.Builder
	for i := range n {
		fmt.Fprintf(&old, "value %d is old\n", i)
		fmt.Fprintf(&new, "value %d is new\n", i)
	}
	return &diffs.Diff{Old: diffs.SplitLines(old.String()), New: diffs.SplitLines(new.String()),
		Hunks: []diffs.Hunk{hunk(1, n, 1, n, block(diffs.Change, 1, n, 1, n))}}
}

func emphasizedTexts(h *harness, th *Theme, side diffs.Side, cell diffs.Cell) []string {
	var out []string
	bg := th.AddedWord
	if side == diffs.Old {
		bg = th.DeletedWord
	}
	for _, s := range h.state.spans(th, side, cell) {
		if s.Background == bg {
			out = append(out, s.Text)
		}
	}
	return out
}

// With a runner, emphasis is computed off the frame, once per pair of
// texts, and a reread that keeps a pair's texts keeps its emphasis.
func TestEmphasisRunsOffTheFrameAndSurvivesRereads(t *testing.T) {
	q := &queue{}
	th := DefaultTheme(&ui.Theme{Background: ui.Hex("#ffffff"), Text: ui.Hex("#333333"), Success: ui.Hex("#13703f"), Danger: ui.Hex("#bd3029")})
	h := newHarnessWith(t, changedPairs(3), &State{Style: diffs.Split, Theme: th, Run: q.run})
	if len(q.work) != 3 {
		t.Fatalf("%d jobs for 3 pairs in view", len(q.work))
	}
	cell := h.state.Rows()[0].New
	if got := emphasizedTexts(h, th, diffs.New, cell); got != nil {
		t.Errorf("emphasis %q before its job ran", got)
	}
	h.tt.Frame()
	if len(q.work) != 3 {
		t.Errorf("a frame queued %d more jobs for pending pairs", len(q.work)-3)
	}
	q.drain()
	h.tt.Frame()
	if got := emphasizedTexts(h, th, diffs.New, cell); len(got) != 1 || got[0] != "new" {
		t.Errorf("emphasis %q after its job ran; want [new]", got)
	}
	// A reread: the first pair's texts stay, the second's change.
	next := changedPairs(3)
	next.New[1] = "value 1 is newer\n"
	h.state.Update(next)
	h.tt.Frame()
	if len(q.work) != 1 {
		t.Errorf("%d jobs after a reread changing one pair", len(q.work))
	}
	if got := emphasizedTexts(h, th, diffs.New, h.state.Rows()[0].New); len(got) != 1 || got[0] != "new" {
		t.Errorf("a kept pair lost its emphasis: %q", got)
	}
}

// Host colors are asked for once per line until SpansVersion changes, and
// not for lines of MaxHighlightLength or more.
func TestStyledLinesAreCached(t *testing.T) {
	d := changedPairs(2)
	d.New[1] = strings.Repeat("x", MaxHighlightLength) + "\n"
	calls := map[diffs.Side]map[int]int{diffs.Old: {}, diffs.New: {}}
	red := ui.Hex("#ff0000")
	h := newHarnessWith(t, d, &State{Style: diffs.Split, SpansVersion: 1, Spans: func(side diffs.Side, line int) []Span {
		calls[side][line]++
		return []Span{{End: 5, Color: red}}
	}})
	for range 2 {
		h.tt.Frame()
	}
	if calls[diffs.New][1] != 1 || calls[diffs.Old][1] != 1 {
		t.Errorf("colors asked %v over three frames; want once per line", calls)
	}
	if calls[diffs.New][2] != 0 {
		t.Errorf("colors asked for a line of %d characters", MaxHighlightLength)
	}
	h.state.SpansVersion = 2
	h.tt.Frame()
	if calls[diffs.New][1] != 2 {
		t.Errorf("new SpansVersion asked %d times; want 2", calls[diffs.New][1])
	}
}

// styleLine cuts at every color and emphasis bound, in one pass.
func TestStyleLineMergesColorsAndWords(t *testing.T) {
	red, blue, bg := ui.Hex("#ff0000"), ui.Hex("#0000ff"), ui.Hex("#00ff00")
	spans := styleLine("abcdefgh", []Span{{End: 3, Color: red}, {End: 8, Color: blue}}, []diffs.Range{{From: 2, To: 5}, {From: 7, To: 20}}, bg, 4)
	var got []string
	for _, s := range spans {
		mark := ""
		if s.Background == bg {
			mark = "*"
		}
		color := map[ui.Color]string{red: "r", blue: "b"}[s.Color]
		got = append(got, color+mark+s.Text)
	}
	if want := "rab r*c b*de bfg b*h"; strings.Join(got, " ") != want {
		t.Errorf("styleLine = %q; want %q", strings.Join(got, " "), want)
	}
}

// Wide characters take two columns, for tab stops and the row's width.
func TestWideCharactersTakeTwoColumns(t *testing.T) {
	col := 0
	if got := expandTabs("日\tx", &col, 4); got != "日  x" || col != 5 {
		t.Errorf("expandTabs = %q, column %d; want \"日  x\", 5", got, col)
	}
	if got := columns([]string{"ab\n", "日本語\n", "😀é\n"}, 2); got != 6 {
		t.Errorf("columns = %d; want 6", got)
	}
	if got := expandedWidth("😀é", 2); got != 3 {
		t.Errorf("emoji and a combining accent take %d columns; want 3", got)
	}
}

// manySpans is a line of n one-byte colored runs, a minified file's.
func manySpans(n int) (string, []Span) {
	spans := make([]Span, n)
	for i := range spans {
		spans[i] = Span{End: i + 1, Color: ui.Hex("#ff0000")}
	}
	return strings.Repeat("x", n), spans
}

func BenchmarkStyleLine20000Spans(b *testing.B) {
	text, spans := manySpans(20000)
	words := []diffs.Range{{From: 100, To: 200}, {From: 5000, To: 9000}}
	for b.Loop() {
		styleLine(text, spans, words, ui.Hex("#00ff00"), 2)
	}
}

// A frame of 40 changed pairs that share no word, scrolled into view:
// emphasis is bounded per pair, and cached after the first frame.
func BenchmarkFramePathologicalPairs(b *testing.B) {
	var old, new strings.Builder
	for i := range 40 {
		for j := 0; j < 160; j++ {
			fmt.Fprintf(&old, "a%d ", (i+j)%10)
			fmt.Fprintf(&new, "B%d ", (i*7+j)%10)
		}
		old.WriteString("\n")
		new.WriteString("\n")
	}
	d := &diffs.Diff{Old: diffs.SplitLines(old.String()), New: diffs.SplitLines(new.String()),
		Hunks: []diffs.Hunk{hunk(1, 40, 1, 40, block(diffs.Change, 1, 40, 1, 40))}}
	state := &State{Style: diffs.Split}
	state.SetDiff(d)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { View(c, state).Grow(1) })
	}, 900, 1000)
	b.Run("first", func(b *testing.B) {
		for b.Loop() {
			state.emphasis, state.styled = nil, nil
			tt.Frame()
		}
	})
	b.Run("cached", func(b *testing.B) {
		for b.Loop() {
			tt.Frame()
		}
	})
}
