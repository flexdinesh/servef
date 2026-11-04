package mygo_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// packages/mygo is generic agent tooling for any mygo app (decision 0086):
// it imports nothing from servef, though its module path sits under it.
func TestImportsNothingFromServef(t *testing.T) {
	const servef, self = "github.com/flexdinesh/servef/", "github.com/flexdinesh/servef/packages/mygo"
	files := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		files++
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(imported, servef) && imported != self && !strings.HasPrefix(imported, self+"/") {
				t.Errorf("%s imports %s; packages/mygo imports nothing from servef", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no Go files found")
	}
}
