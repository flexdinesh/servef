package document

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type familyFixture struct {
	ID       string `json:"id"`
	Language string `json:"language"`
}

// TestDocumentFamiliesFixtureMatchesBuiltins keeps testdata/document-families.json
// in step with Builtins(). The web tests read each family's language from that
// fixture and check it is a Highlights language (decision 0029).
func TestDocumentFamiliesFixtureMatchesBuiltins(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "document-families.json"))
	if err != nil {
		t.Fatal(err)
	}
	var families []familyFixture
	if err := json.Unmarshal(data, &families); err != nil {
		t.Fatal(err)
	}
	want := make([]familyFixture, 0, len(families))
	for _, fileType := range Builtins().Types() {
		want = append(want, familyFixture{ID: fileType.ID, Language: fileType.Language})
	}
	if !reflect.DeepEqual(families, want) {
		t.Fatalf("testdata/document-families.json = %+v\nwant Builtins() families %+v\nupdate the fixture; the web tests check each language is a Highlights language (decision 0029)", families, want)
	}
}
