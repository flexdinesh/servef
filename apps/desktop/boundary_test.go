package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The app is a client of servef (decision 0094): from the servef module it
// imports only the transport contract and the Go client. Its module path
// sits under servef's, so Go's internal rule would not stop it.
func TestImportsOnlyServefContractAndClient(t *testing.T) {
	const servef = "github.com/flexdinesh/servef/"
	allowed := map[string]bool{servef + "pkg/contract": true, servef + "pkg/client": true}
	files := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (path == "build" || path == "resources") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
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
			if strings.HasPrefix(imported, servef) && !strings.HasPrefix(imported, servef+"apps/desktop/") && !allowed[imported] {
				t.Errorf("%s imports %s; the app imports only pkg/contract and pkg/client from servef", path, imported)
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
