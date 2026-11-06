package web

import (
	"io/fs"
	"testing"

	"github.com/flexdinesh/srcz/internal/app"
	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// Messages the Source Control tests expect from the application layer.
const (
	commitUnavailableMessage     = app.CommitUnavailableMessage
	invalidCommitMessage         = app.InvalidCommitMessage
	changeUnavailableMessage     = app.ChangeUnavailableMessage
	commitFileUnavailableMessage = app.CommitFileUnavailableMessage
	unknownBaseMessage           = app.UnknownBaseMessage
	unknownHeadMessage           = app.UnknownHeadMessage
)

// documentType is the contract form of a path's registered type.
func documentType(a *App, name string) contract.FileType {
	return fileTypeContract(a.config.Types.Lookup(name).Type())
}

func fileTypeContract(fileType document.Type) contract.FileType {
	return contract.FileType{
		ID:           fileType.ID,
		Label:        fileType.Label,
		Extensions:   fileType.Extensions,
		Filenames:    fileType.Filenames,
		Language:     fileType.Language,
		PreviewKind:  contract.PreviewKind(fileType.PreviewKind),
		PreviewLabel: fileType.PreviewLabel,
		DefaultMode:  contract.DefaultDocumentMode(fileType.DefaultMode),
		Formattable:  fileType.Formattable,
	}
}

// withStatsBudget rebuilds a's application layer with a compare-stats read
// budget.
func withStatsBudget(t *testing.T, a *App, budget int64) {
	t.Helper()
	application, err := app.New(app.Config{
		Roots: a.roots, ScanDepth: a.config.ScanDepth, Depth: a.config.Depth, Types: a.config.Types,
		OpenFile:    func(name string) (fs.File, error) { return a.openFile(name) },
		StatsBudget: budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	a.app = application
}
