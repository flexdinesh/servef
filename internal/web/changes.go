package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
)

const (
	// changesLimit caps the changes one status response lists.
	changesLimit = 5000
	// statusTimeout bounds one status read.
	statusTimeout = 20 * time.Second

	changeUnavailableMessage = "That change is unavailable."
	// unchangedCode marks a 404 for a path with no changes to diff.
	unchangedCode = "unchanged"
)

// Source Control is repository-scoped: it lists every change in the
// project's checkout, whatever the CLI filename scope, and reads worktree
// files only after the git package validates them as changes.

type changesResponse struct {
	Repository bool         `json:"repository"`
	Branch     string       `json:"branch"`
	Files      []git.Change `json:"files"`
	Truncated  bool         `json:"truncated"`
	Warnings   []string     `json:"warnings"`
}

// diffSide is one side of a diff under the 0004 limits. Content is absent for
// binary and too-large sides.
type diffSide struct {
	Content  *string `json:"content,omitempty"`
	Size     int64   `json:"size"`
	Binary   bool    `json:"binary"`
	TooLarge bool    `json:"tooLarge"`
}

// diffResponse pairs HEAD's side (Original) with the worktree's (Modified);
// a side the change does not have is null.
type diffResponse struct {
	Path         string        `json:"path"`
	OldPath      string        `json:"oldPath,omitempty"`
	Status       git.Status    `json:"status"`
	DocumentType document.Type `json:"documentType"`
	Original     *diffSide     `json:"original"`
	Modified     *diffSide     `json:"modified"`
}

type diffErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func (a *App) serveChanges(w http.ResponseWriter, r *http.Request, projectID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	response := changesResponse{Files: []git.Change{}, Warnings: []string{}}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git status: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()
	changes, err := repo.Status(ctx, git.StatusOptions{Limit: changesLimit})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git status: " + err.Error()})
		return
	}
	response.Repository = true
	response.Branch = repo.Branch()
	response.Files = changes.Files
	response.Truncated = changes.Truncated
	response.Warnings = warningStrings(changes.Warnings)
	writeJSON(w, http.StatusOK, response)
}

func (a *App) serveDiff(w http.ResponseWriter, r *http.Request, projectID, name, oldPath string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "No file was selected."})
		return
	}
	repo, err := git.Open(project.root)
	if errors.Is(err, git.ErrNotRepository) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git status: " + err.Error()})
		return
	}
	defer func() { _ = repo.Close() }()

	entry, err := repo.Lookup(r.Context(), name, oldPath)
	switch {
	case errors.Is(err, git.ErrUnchanged):
		writeJSON(w, http.StatusNotFound, diffErrorResponse{Error: fmt.Sprintf("No changes to %s since the last commit.", name), Code: unchangedCode})
		return
	case errors.Is(err, git.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: changeUnavailableMessage})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Couldn't read git status: " + err.Error()})
		return
	}

	fileType := a.config.Types.Lookup(name).Type()
	response := diffResponse{Path: entry.Path, OldPath: entry.OldPath, Status: entry.Status, DocumentType: fileType}
	if entry.Status != git.Added && entry.Status != git.Untracked {
		original := entry.Path
		if entry.OldPath != "" {
			original = entry.OldPath
		}
		blob, err := repo.HeadBlob(original)
		if err == nil {
			response.Original, err = readDiffSide(blob.Size, blob.Open, fileType)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s from the last commit: %v", original, err)})
			return
		}
	}
	if entry.Status != git.Deleted {
		response.Modified, err = a.readWorktreeSide(project.root, entry, fileType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("Could not read %s: %v", entry.Path, err)})
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// readWorktreeSide reads a validated change's worktree file: a symlink's
// target text, or the root-confined regular file.
func (a *App) readWorktreeSide(root string, entry git.Entry, fileType document.Type) (*diffSide, error) {
	if entry.Symlink {
		link := []byte(entry.Link)
		return readDiffSide(int64(len(link)), func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(link)), nil
		}, fileType)
	}
	resolved, err := files.ResolveFile(root, entry.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	return readDiffSide(info.Size(), func() (io.ReadCloser, error) { return a.openFile(resolved) }, fileType)
}

// readDiffSide applies the 0004 limits to one side of size bytes. Sides over
// DisplayLimit and types without a text source are never opened; binary and
// over-PreviewLimit sides read only the sniffed prefix.
func readDiffSide(size int64, open func() (io.ReadCloser, error), fileType document.Type) (*diffSide, error) {
	side := &diffSide{Size: size, TooLarge: size > document.PreviewLimit}
	if !fileType.HasTextSource() {
		side.Binary = true
		return side, nil
	}
	if size > document.DisplayLimit {
		return side, nil
	}
	reader, err := open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	// Too-large sides only need the prefix to report binary.
	limit := int64(document.PreviewLimit)
	if side.TooLarge {
		limit = document.SniffLength
	}
	text, binary, err := readText(reader, limit)
	if err != nil {
		return nil, err
	}
	if binary {
		side.Binary = true
		return side, nil
	}
	if side.TooLarge {
		return side, nil
	}
	content := string(text)
	side.Content = &content
	return side, nil
}
