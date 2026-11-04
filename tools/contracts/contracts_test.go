package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repositoryRoot = "../.."

func repositoryModel(t *testing.T) *model {
	t.Helper()
	m, err := load(filepath.Join(repositoryRoot, sourceDir), generated, goOnly)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestAPIFixturesSatisfyGeneratedContracts validates the frontend fixtures
// of generated shapes against the model the TypeScript guards come from:
// fields, enums, and rules, with unknown keys rejected.
func TestAPIFixturesSatisfyGeneratedContracts(t *testing.T) {
	m := repositoryModel(t)
	shapes := map[string]string{
		"projects.json":          "ProjectsResponse",
		"directories.json":       "DirectoriesResponse",
		"page-*.json":            "PageData",
		"format-settings.json":   "FormatResult",
		"format-error.json":      "ErrorResponse",
		"error-*.json":           "ErrorResponse",
		"search-documents.json":  "SearchDocumentsResponse",
		"changes*.json":          "ChangesResponse",
		"diff-*.json":            "DiffResponse",
		"commits*.json":          "CommitsResponse",
		"commit-files.json":      "CommitFilesResponse",
		"branch-files*.json":     "BranchFilesResponse",
		"range-files.json":       "RangeFilesResponse",
		"refs.json":              "RefsResponse",
		"branch-stack.json":      "StackResponse",
		"compare-stats*.json":    "CompareStatsResponse",
		"metrics*.json":          "ProcessMetrics",
		"navigation-status.json": "NavigationStatus",
		"definition.json":        "NavigationResult",
		"references.json":        "NavigationResult",
		"walkthrough-tour.json":  "Walkthrough",
		"walkthroughs.json":      "[]WalkthroughSummary",
	}
	checked := 0
	for pattern, typeName := range shapes {
		names, err := filepath.Glob(filepath.Join(repositoryRoot, "testdata", "api", "default", pattern))
		if err != nil || len(names) == 0 {
			t.Fatalf("%s matches %v, %v", pattern, names, err)
		}
		for _, name := range names {
			content, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(content, &value); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			// A []T shape is a bare array of T.
			if elem, ok := strings.CutPrefix(typeName, "[]"); ok {
				err = m.validateRef(typeRef{kind: arrayRef, elem: &typeRef{kind: namedRef, name: elem}}, value, "$")
			} else {
				err = m.validate(typeName, value)
			}
			if err != nil {
				t.Errorf("%s as %s: %v", filepath.Base(name), typeName, err)
			}
			checked++
		}
	}
	if checked < 37 {
		t.Errorf("checked %d fixtures, want every generated shape's", checked)
	}
}

func TestValidateRejectsBrokenRulesAndShapes(t *testing.T) {
	m := repositoryModel(t)
	content, err := os.ReadFile(filepath.Join(repositoryRoot, "testdata", "api", "default", "page-document.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(map[string]any){
		"unknown field":     func(page map[string]any) { page["extra"] = true },
		"missing field":     func(page map[string]any) { delete(page, "tooLarge") },
		"wrong type":        func(page map[string]any) { page["binary"] = "no" },
		"unknown enum":      func(page map[string]any) { page["projectKind"] = "submodule" },
		"file without type": func(page map[string]any) { page["documentType"] = nil },
		"preview without label": func(page map[string]any) {
			page["documentType"].(map[string]any)["previewLabel"] = ""
		},
	}
	for name, mutate := range cases {
		var page map[string]any
		if err := json.Unmarshal(content, &page); err != nil {
			t.Fatal(err)
		}
		if err := m.validate("PageData", page); err != nil {
			t.Fatalf("unmodified page: %v", err)
		}
		mutate(page)
		if err := m.validate("PageData", page); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func writeContract(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGeneratorEmitsGuardsWithRulesAndImports(t *testing.T) {
	dir := writeContract(t, map[string]string{
		"a.go": `package contract

// Mode is a mode.
type Mode string

const (
	ModeOn  Mode = "on"
	ModeOff Mode = "off"
)
`,
		"b.go": `package contract

// Thing is a thing.
//
//contract:rule enabled == true => name != ""
//contract:rule enabled == true => note != ""
type Thing struct {
	// Name names it.
	Name    string   ` + "`json:\"name\"`" + `
	Mode    Mode     ` + "`json:\"mode\"`" + `
	Enabled bool     ` + "`json:\"enabled\"`" + `
	Tags    []string ` + "`json:\"tags\"`" + `
	Next    *Thing   ` + "`json:\"next\"`" + `
	Note    string   ` + "`json:\"note,omitempty\"`" + `
	Count   *int     ` + "`json:\"count,omitempty\"`" + `
	Labels  []string ` + "`json:\"labels,omitempty\"`" + `
	Ratio   *float64 ` + "`json:\"ratio\"`" + `
	Parent  *Thing   ` + "`json:\"parent,omitempty\"`" + `
	Tone    Mode     ` + "`json:\"tone,omitempty\"`" + `
	Skip    string   ` + "`json:\"-\"`" + `
}
`,
	})
	m, err := load(dir, []string{"a.go", "b.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := emit(m)
	b := output["b.ts"]
	for _, want := range []string{
		`import { isArrayOf, isBoolean, isNumber, isRecord, isString } from "./guards.ts"`,
		`import { type Mode, isMode } from "./a.ts"`,
		"  // name names it.\n  name: string",
		"  next: Thing | null",
		"  note?: string",
		"  count?: number",
		"  labels?: string[]",
		"  ratio: number | null",
		"  parent?: Thing",
		"  tone?: Mode",
		"&& (value.parent === undefined || isThing(value.parent))",
		"&& (value.tone === undefined || isMode(value.tone))",
		"&& (value.labels === undefined || isArrayOf(value.labels, isString))",
		"&& (value.ratio === null || isNumber(value.ratio))",
		"&& (value.count === undefined || isNumber(value.count))",
		`&& (value.enabled !== true || (value.note ?? "") !== "")`,
		"&& (value.next === null || isThing(value.next))",
		"&& (value.note === undefined || isString(value.note))",
		`&& (value.enabled !== true || value.name !== "")`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("b.ts lacks %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "skip") {
		t.Errorf("b.ts emits a json:\"-\" field:\n%s", b)
	}
	if want := `export type Mode = "on" | "off"`; !strings.Contains(output["a.ts"], want) {
		t.Errorf("a.ts lacks %q:\n%s", want, output["a.ts"])
	}
	if want := "export * from \"./a.ts\"\nexport * from \"./b.ts\"\n"; !strings.HasSuffix(output["index.ts"], want) {
		t.Errorf("index.ts = %q", output["index.ts"])
	}
}

func TestGeneratorRejectsWhatGuardsCannotExpress(t *testing.T) {
	cases := map[string]string{
		"untagged field":      "type T struct {\n\tName string\n}\n",
		"map field":           "type T struct {\n\tM map[string]string `json:\"m\"`\n}\n",
		"unknown rule":        "//contract:rule missing == true => name != \"\"\ntype T struct {\n\tName string `json:\"name\"`\n}\n",
		"malformed rule":      "//contract:rule name is set\ntype T struct {\n\tName string `json:\"name\"`\n}\n",
		"function":            "func F() {}\n",
		"untyped const":       "const C = \"c\"\n",
		"unknown named":       "type T struct {\n\tU Unknown `json:\"u\"`\n}\n",
		"optional number":     "type T struct {\n\tN int `json:\"n,omitempty\"`\n}\n",
		"optional struct":     "type U struct{}\ntype T struct {\n\tU U `json:\"u,omitempty\"`\n}\n",
		"optional enum ptr":   "type E string\n\nconst EOn E = \"on\"\n\ntype T struct {\n\tE *E `json:\"e,omitempty\"`\n}\n",
		"optional ptr rule":   "type U struct{}\n\n//contract:rule u == null => s != \"\"\ntype T struct {\n\tU *U `json:\"u,omitempty\"`\n\tS string `json:\"s\"`\n}\n",
		"optional rule":       "//contract:rule n == true => s != \"\"\ntype T struct {\n\tN *bool `json:\"n,omitempty\"`\n\tS string `json:\"s\"`\n}\n",
		"optional slice rule": "//contract:rule n == null => s != \"\"\ntype T struct {\n\tN []string `json:\"n,omitempty\"`\n\tS string `json:\"s\"`\n}\n",
		"pointer to slice":    "type T struct {\n\tN *[]string `json:\"n\"`\n}\n",
	}
	for name, body := range cases {
		dir := writeContract(t, map[string]string{"a.go": "package contract\n\n" + body})
		if _, err := load(dir, []string{"a.go"}, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	dir := writeContract(t, map[string]string{"a.go": "package contract\n", "b.go": "package contract\n"})
	if _, err := load(dir, []string{"a.go"}, nil); err == nil {
		t.Error("an unlisted contract file was accepted")
	}
	if _, err := load(dir, []string{"a.go", "b.go", "c.go"}, nil); err == nil {
		t.Error("a listed but missing contract file was accepted")
	}
}

func TestCheckReportsDrift(t *testing.T) {
	source := writeContract(t, map[string]string{"a.go": "package contract\n\n// T is t.\ntype T struct {\n\tName string `json:\"name\"`\n}\n"})
	m, err := load(source, []string{"a.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := emit(m)
	dir := t.TempDir()
	for name, content := range output {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := compare(dir, output); err != nil {
		t.Fatalf("current output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.ts"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.ts"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err = compare(dir, output)
	if err == nil || !strings.Contains(err.Error(), "a.ts") || !strings.Contains(err.Error(), "stray.ts (not generated)") {
		t.Errorf("drift = %v, want a.ts and stray.ts", err)
	}
}
