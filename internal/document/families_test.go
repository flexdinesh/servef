package document

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestDocumentFamiliesFixtureMatchesBuiltins keeps testdata/document-families.json
// in step with Builtins(). The web build checks that fixture so every family
// has an explicit diff grammar choice in web/src/diff-language.ts (decision 0024).
func TestDocumentFamiliesFixtureMatchesBuiltins(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "document-families.json"))
	if err != nil {
		t.Fatal(err)
	}
	var families []string
	if err := json.Unmarshal(data, &families); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(families))
	for _, fileType := range Builtins().Types() {
		want = append(want, fileType.ID)
	}
	if !reflect.DeepEqual(families, want) {
		t.Fatalf("testdata/document-families.json = %q\nwant Builtins() IDs %q\nupdate the fixture and map new families in web/src/diff-language.ts", families, want)
	}
}
