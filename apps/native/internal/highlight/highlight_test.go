package highlight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/packages/mygo/highlights"
)

var repo = filepath.Join("..", "..", "..", "..")

// categoryAt returns the category covering byte offset in line.
func categoryAt(line []Span, offset int) Category {
	for _, span := range line {
		if offset < span.End {
			return span.Category
		}
	}
	return Plain
}

func TestLinesGo(t *testing.T) {
	text := "package main\n\n// Greet says hi.\nfunc Greet(n int) string {\n\treturn \"hi\" + 42\n}"
	lines := Lines(text, "go")
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6", len(lines))
	}
	for _, tc := range []struct {
		line   int
		needle string
		want   Category
	}{
		{0, "package", Keyword},
		{2, "// Greet", Comment},
		{3, "func", Keyword},
		{3, "Greet", Function},
		{3, "int", Type},
		{4, "\"hi\"", String},
		{4, "42", Number},
		{4, "+", Operator},
	} {
		source := strings.Split(text, "\n")[tc.line]
		offset := strings.Index(source, tc.needle)
		if got := categoryAt(lines[tc.line], offset); got != tc.want {
			t.Errorf("line %d %q: %v, want %v (spans %v)", tc.line, tc.needle, got, tc.want, lines[tc.line])
		}
	}
	if lines[1] != nil {
		t.Errorf("empty line has spans %v", lines[1])
	}
}

func TestLinesPlain(t *testing.T) {
	lines := Lines("a\nbb\n", "text")
	want := [][]Span{{{End: 1}}, {{End: 2}}, nil}
	if len(lines) != len(want) {
		t.Fatalf("lines %v", lines)
	}
	for i := range want {
		if len(lines[i]) != len(want[i]) || (len(want[i]) > 0 && lines[i][0] != want[i][0]) {
			t.Fatalf("line %d: %v, want %v", i, lines[i], want[i])
		}
	}
	for _, tc := range [][2]string{{"x := 1", "klingon"}, {"a\rb := 1", "go"}} {
		if got := Lines(tc[0], tc[1]); len(got) != 1 || len(got[0]) != 1 || got[0][0].Category != Plain {
			t.Fatalf("%q in %s is plain: %v", tc[0], tc[1], got)
		}
	}
}

func TestLinesKeepLineCount(t *testing.T) {
	for _, text := range []string{"", "\n", "a\r\nb\r\n", "x = 1\n\n\n"} {
		if got, want := len(Lines(text, "python")), strings.Count(text, "\n")+1; got != want {
			t.Errorf("%q: %d lines, want %d", text, got, want)
		}
	}
	// A CRLF line's "\r" stays unstyled after its spans.
	lines := Lines("x = 1\r\ny", "python")
	if last := lines[0][len(lines[0])-1]; last.End != len("x = 1") {
		t.Fatalf("spans %v", lines[0])
	}
}

// run is a stretch of text in one hl- class list.
type run struct{ text, class string }

func merged(runs []run) []run {
	var out []run
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].class == r.class {
			out[n-1].text += r.text
			continue
		}
		out = append(out, r)
	}
	return out
}

var spanPattern = regexp.MustCompile(`<span class="([^"]*)">([^<]*)</span>|([^<]+)`)

// webRuns parses a row of syntax-highlight.ts's HTML.
func webRuns(row string) []run {
	var runs []run
	for _, m := range spanPattern.FindAllStringSubmatch(row, -1) {
		if m[3] != "" {
			runs = append(runs, run{text: html.UnescapeString(m[3])})
		} else {
			runs = append(runs, run{text: html.UnescapeString(m[2]), class: m[1]})
		}
	}
	return merged(runs)
}

// className is tokenClass's class list for a category.
func className(c Category) string {
	var classes []string
	if name := c.Name(); name != "" {
		classes = append(classes, "hl-"+name)
	}
	if c.Italic() {
		classes = append(classes, "hl-italic")
	}
	if c.Bold() {
		classes = append(classes, "hl-bold")
	}
	return strings.Join(classes, " ")
}

func nativeRuns(line string, spans []Span) []run {
	var runs []run
	start := 0
	for _, span := range spans {
		runs = append(runs, run{text: line[start:span.End], class: className(span.Category)})
		start = span.End
	}
	runs = append(runs, run{text: line[start:]})
	return merged(runs)
}

type webGolden struct {
	Inputs map[string]string   `json:"inputs"`
	Rows   map[string][]string `json:"rows"`
}

func readWebGolden(t *testing.T) webGolden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "web-rows.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g webGolden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// Every sample shows the classes the web app renders for it
// (testdata/web-rows.json, from scripts/web-highlights.ts): the same
// token boundaries and roles, the same plain fallbacks.
func TestMatchesWebRows(t *testing.T) {
	g := readWebGolden(t)
	if len(g.Rows) == 0 {
		t.Fatal("no rows")
	}
	samples := filepath.Join(repo, "packages", "mygo", "highlights", "testdata", "samples")
	for file, rows := range g.Rows {
		src, err := os.ReadFile(filepath.Join(samples, file))
		if err != nil {
			t.Fatal(err)
		}
		language := file[strings.LastIndexByte(file, '.')+1:]
		text := string(src)
		lines := Lines(text, language)
		sources := strings.Split(text, "\n")
		if rows == nil {
			for i, line := range lines {
				if got := nativeRuns(sources[i], line); len(got) > 1 || (len(got) == 1 && got[0].class != "") {
					t.Errorf("%s line %d: %v, want plain", file, i, got)
				}
			}
			continue
		}
		// The web drops the row of a final line break.
		if n := len(lines); n > 1 && n == len(rows)+1 && sources[n-1] == "" {
			lines, sources = lines[:n-1], sources[:n-1]
		}
		if len(lines) != len(rows) {
			t.Errorf("%s: %d lines, web %d rows", file, len(lines), len(rows))
			continue
		}
		for i, row := range rows {
			line := strings.TrimSuffix(sources[i], "\r")
			got, want := nativeRuns(line, lines[i]), webRuns(row)
			if len(got) != len(want) {
				t.Errorf("%s line %d:\n got  %q\n want %q", file, i, got, want)
				break
			}
			for j := range got {
				if got[j] != want[j] {
					t.Errorf("%s line %d:\n got  %q\n want %q", file, i, got, want)
					break
				}
			}
		}
	}
}

// The goldens come from the web files this package mirrors; a change
// there needs them regenerated (and the mirror checked).
func TestWebInputsUnchanged(t *testing.T) {
	g := readWebGolden(t)
	for path, want := range g.Inputs {
		data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s changed: update package highlight if needed and run node apps/native/scripts/web-highlights.ts", path)
		}
	}
}

// syntaxTheme is SYNTAX_THEME's syntax, entry for entry.
func TestSyntaxThemeMirrorsWeb(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repo, "apps", "web", "src", "highlighting", "syntax-theme.ts"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	body := src[strings.Index(src, "syntax: {")+len("syntax: {"):]
	body = body[:strings.Index(body, "\n    },")]
	entry := regexp.MustCompile(`(?m)^\s*"?([a-z_.]+)"?: (?:"([a-z]+)"|\{([^}]*)\}),?$`)
	web := map[string]highlights.Setting{}
	for _, m := range entry.FindAllStringSubmatch(body, -1) {
		var s highlights.Setting
		if m[2] != "" {
			s.Color = m[2]
		} else {
			for _, field := range strings.Split(m[3], ",") {
				name, value, _ := strings.Cut(strings.TrimSpace(field), ": ")
				value = strings.Trim(value, `"`)
				switch name {
				case "color":
					s.Color = value
				case "font_style":
					s.FontStyle = value
				case "font_weight":
					s.FontWeight, _ = strconv.Atoi(value)
				}
			}
		}
		web[m[1]] = s
	}
	if len(web) != len(syntaxTheme) {
		t.Errorf("syntax-theme.ts has %d entries, native %d", len(web), len(syntaxTheme))
	}
	for scope, want := range web {
		if got := syntaxTheme[scope]; got != want {
			t.Errorf("%s: native %+v, web %+v", scope, got, want)
		}
	}
}

// Every scope resolves to a category native can show: a role with the
// web's font settings (comments alone italic), or plain text in italics
// or bold.
func TestEveryScopeHasACategory(t *testing.T) {
	for _, scope := range highlights.Scopes()[1:] {
		setting, _ := highlights.Resolve(syntaxTheme, scope)
		c := categoryOf(syntaxTheme, scope)
		italic, bold := setting.FontStyle == "italic", setting.FontWeight >= 600
		if c.Italic() != italic || c.Bold() != bold {
			t.Errorf("%s resolves to %+v, shown as %v", scope, setting, className(c))
		}
	}
}

// Every language servef declares highlights natively and has a sample.
func TestEveryDeclaredLanguage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repo, "testdata", "document-families.json"))
	if err != nil {
		t.Fatal(err)
	}
	var families []struct {
		ID       string `json:"id"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal(raw, &families); err != nil {
		t.Fatal(err)
	}
	g := readWebGolden(t)
	for _, family := range families {
		if family.Language == "" || family.Language == "text" {
			continue
		}
		if !Supported(family.Language) {
			t.Errorf("family %s declares %q, which native does not highlight", family.ID, family.Language)
		}
		found := false
		for file := range g.Rows {
			if highlights.SameLexer(file[strings.LastIndexByte(file, '.')+1:], family.Language) {
				found = true
			}
		}
		if !found {
			t.Errorf("no sample in %s for family %s", "packages/mygo/highlights/testdata/samples", family.ID)
		}
	}
}

func TestCategoryNames(t *testing.T) {
	var names []string
	for _, c := range Categories {
		names = append(names, c.Name())
	}
	if got := strings.Join(names, ","); got != "comment,keyword,type,function,variable,string,number,operator" {
		t.Fatalf("names %s", got)
	}
	if Plain.Name() != "" || Emphasis.Name() != "" || !Comment.Italic() || !Strong.Bold() {
		t.Fatal("names and styles")
	}
}
