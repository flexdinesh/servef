package docview

import "testing"

func eq(a, b *LineRange) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// line-selection.test.ts.
func TestParseLineAnchor(t *testing.T) {
	for hash, want := range map[string]LineRange{
		"L12": {12, 12}, "#L12": {12, 12}, "L12-L20": {12, 20}, "L12-20": {12, 20}, "L20-L12": {12, 20},
	} {
		if got := ParseLineAnchor(hash); !eq(got, &want) {
			t.Errorf("%q: %+v", hash, got)
		}
	}
	for _, hash := range []string{"", "L", "L0", "L01", "L-3", "l12", "L12-", "L12-L", "L1.5", "heading", "L12x", "L99999999999999999999"} {
		if got := ParseLineAnchor(hash); got != nil {
			t.Errorf("%q is no anchor: %+v", hash, got)
		}
	}
}

func TestLineAnchorFormats(t *testing.T) {
	if LineAnchor(LineRange{7, 7}) != "L7" || LineAnchor(LineRange{7, 9}) != "L7-L9" {
		t.Error("format")
	}
	r := LineRange{3, 40}
	if got := ParseLineAnchor(LineAnchor(r)); !eq(got, &r) {
		t.Errorf("round trip %+v", got)
	}
}

func TestClampLines(t *testing.T) {
	if got := ClampLines(&LineRange{2, 9}, 5); !eq(got, &LineRange{2, 5}) {
		t.Errorf("%+v", got)
	}
	if got := ClampLines(&LineRange{5, 5}, 5); !eq(got, &LineRange{5, 5}) {
		t.Errorf("%+v", got)
	}
	if ClampLines(&LineRange{6, 9}, 5) != nil || ClampLines(nil, 5) != nil {
		t.Error("past the end vanishes")
	}
}

func TestStartLineSelection(t *testing.T) {
	for _, c := range []struct {
		current *LineRange
		anchor  int
		line    int
		extend  bool
		from    int
		want    LineRange
	}{
		{nil, 0, 4, false, 4, LineRange{4, 4}},
		{&LineRange{2, 8}, 2, 5, false, 5, LineRange{5, 5}},
		{&LineRange{4, 4}, 4, 9, true, 4, LineRange{4, 9}},
		{&LineRange{4, 9}, 4, 1, true, 4, LineRange{1, 4}},
		// Without a remembered anchor (after a #L10-L12 link), Shift
		// extends from the start.
		{&LineRange{10, 12}, 0, 20, true, 10, LineRange{10, 20}},
		{nil, 0, 3, true, 3, LineRange{3, 3}},
	} {
		from, got := StartLineSelection(c.current, c.anchor, c.line, c.extend)
		if from != c.from || got != c.want {
			t.Errorf("%+v: %d %+v", c, from, got)
		}
	}
}

func TestSpanLines(t *testing.T) {
	if SpanLines(5, 9) != (LineRange{5, 9}) || SpanLines(5, 2) != (LineRange{2, 5}) || SpanLines(5, 5) != (LineRange{5, 5}) {
		t.Error("span")
	}
}

func TestReferencesAndLabels(t *testing.T) {
	if got := LineReference("web/src/App.tsx", LineRange{12, 12}); got != "web/src/App.tsx:12" {
		t.Error(got)
	}
	if got := LineReference("web/src/App.tsx", LineRange{12, 20}); got != "web/src/App.tsx:12-20" {
		t.Error(got)
	}
	if got := LineRangeLabel(LineRange{12, 12}); got != "Ln 12" {
		t.Error(got)
	}
	if got := LineRangeLabel(LineRange{12, 20}); got != "Ln 12–20" {
		t.Error(got)
	}
}

func TestDisplayTextSplitsLinesAsTheWeb(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"a\nb\n", "a\nb\n"},
		{"a\r\nb\r\n", "a\r\nb\r\n"},
		{"a\rb\rc", "a\nb\nc"},
		{"a\r\nb\rc\r", "a\r\nb\nc\n"},
		{"\r\r\n", "\n\r\n"},
	} {
		if got := DisplayText(tc.in); got != tc.want {
			t.Errorf("DisplayText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
