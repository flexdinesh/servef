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

// The app is a client of srcz (decision 0094): from the srcz module it
// imports only the transport contract, the connection model, and the Go
// client. Its module path sits under srcz's, so Go's internal rule would not
// stop it.
func TestImportsOnlySrczContractAndClient(t *testing.T) {
	const srcz = "github.com/flexdinesh/srcz/"
	allowed := map[string]bool{srcz + "pkg/contract": true, srcz + "pkg/connection": true, srcz + "pkg/client": true}
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
			if strings.HasPrefix(imported, srcz) && !strings.HasPrefix(imported, srcz+"apps/desktop/") && !allowed[imported] {
				t.Errorf("%s imports %s; the app imports only pkg/contract, pkg/connection, and pkg/client from srcz", path, imported)
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
