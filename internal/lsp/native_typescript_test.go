package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Run against the build-time native TypeScript installation when present.
// This guards the import-site regression without requiring global tools.
func TestNativeTypeScriptResolvesImportsReexportsAliasesAndSemanticReferences(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	native, ok := nativeTypeScript(filepath.Join(cwd, "..", "..", "node_modules", "typescript"), lookPath)
	if !ok {
		t.Skip("repository-local native TypeScript not installed")
	}
	root := t.TempDir()
	texts := map[string]string{
		"tsconfig.json": `{"compilerOptions":{"target":"ESNext","module":"ESNext","moduleResolution":"bundler","strict":true},"include":["*.ts"]}`,
		"values.ts":     "export class MissingValue extends Error {}\n",
		"index.ts":      "export { MissingValue as DomainError } from './values';\n",
		"errors.ts":     "import { DomainError } from './index';\nexport const error = new DomainError();\nfunction local() { const DomainError = 1; return DomainError; }\n",
	}
	files := make([]File, 0, len(texts))
	for name, text := range texts {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, File{Path: path, Size: info.Size(), Modified: info.ModTime()})
	}
	server := Server{Name: "native-test", Args: native.Args, Families: map[string]string{"typescript": "typescript"}}
	m := NewManager([]Server{server}, Options{LookPath: func(string) (string, bool) { return native.Path, true }})
	t.Cleanup(m.Close)
	request := Request{Root: root, Family: "typescript", Text: []byte(texts["errors.ts"]), At: Position{Line: 1, Column: 12}, Files: files}
	for _, file := range files {
		if filepath.Base(file.Path) == "errors.ts" {
			request.Document = file
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	definitions, err := m.Definition(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || filepath.Base(definitions[0].Path) != "values.ts" || definitions[0].Line != 1 {
		t.Fatalf("import definition = %+v", definitions)
	}
	request.Document = File{}
	for _, file := range files {
		if filepath.Base(file.Path) == "values.ts" {
			request.Document = file
		}
	}
	request.Text = []byte(texts["values.ts"])
	request.At = Position{Line: 1, Column: 15}
	references, err := m.References(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	foundImport, foundUse := false, false
	for _, reference := range references {
		if filepath.Base(reference.Path) == "errors.ts" {
			if reference.Line == 3 {
				t.Fatalf("shadowed local included in semantic references: %+v", references)
			}
			foundImport = foundImport || reference.Line == 1
			foundUse = foundUse || reference.Line == 2
		}
	}
	if !foundImport || !foundUse {
		t.Fatalf("missing alias references: %+v", references)
	}
}
