package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/internal/projects"
)

type pageData struct {
	Roots   []string `json:"roots"`
	Project string   `json:"project"`
	// ProjectUnavailable reports that the project itself could not be
	// resolved, as opposed to a file within it.
	ProjectUnavailable bool          `json:"projectUnavailable"`
	RootName           string        `json:"rootName"`
	Repository         string        `json:"repository"`
	ProjectKind        projects.Kind `json:"projectKind"`
	Branch             string        `json:"branch"`
	Worktrees          int           `json:"worktrees"`
	Tree               []treeNode    `json:"tree"`
	Selected           string        `json:"selected"`
	// At is the full commit hash the selected document was read from, or
	// empty for the working tree.
	At                  string         `json:"at"`
	Content             string         `json:"content"`
	Source              string         `json:"source"`
	FileSize            int64          `json:"fileSize"`
	FileCount           int            `json:"fileCount"`
	HasFile             bool           `json:"hasFile"`
	Empty               bool           `json:"empty"`
	Error               string         `json:"error"`
	Warnings            []string       `json:"warnings"`
	FilenamePatterns    []string       `json:"filenamePatterns"`
	DocumentType        *document.Type `json:"documentType"`
	NavigationSupported bool           `json:"navigationSupported"`
	Binary              bool           `json:"binary"`
	TooLarge            bool           `json:"tooLarge"`
	PreviewDisabled     bool           `json:"previewDisabled"`
	HighlightDisabled   bool           `json:"highlightDisabled"`
	SearchExcluded      bool           `json:"searchExcluded"`
	PreviewError        string         `json:"previewError"`
}

type treeNode struct {
	FileType string     `json:"fileType"`
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Open     bool       `json:"open"`
	Selected bool       `json:"selected"`
	Children []treeNode `json:"children"`
}

type searchDocument struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type searchDocumentsResponse struct {
	Documents []searchDocument `json:"documents"`
	Warnings  []string         `json:"warnings"`
}

type formatResponse struct {
	Formatted string `json:"formatted"`
}

// indexedFile is a read of an indexed file. Binary files keep only the sniffed
// prefix; files over the read limit are not read.
type indexedFile struct {
	source   []byte
	size     int64
	binary   bool
	tooLarge bool
}

// readIndexed reads a resolved file unless it is larger than limit (when
// limit >= 0). Binary files stop after the sniffed prefix; types without a text
// source are binary without reading content.
func (a *App) readIndexed(resolved string, fileType document.Type, limit int64) (indexedFile, error) {
	file, err := a.openFile(resolved)
	if err != nil {
		return indexedFile{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return indexedFile{}, err
	}
	result := indexedFile{size: info.Size()}
	if limit >= 0 && result.size > limit {
		result.tooLarge = true
		return result, nil
	}
	if !fileType.HasTextSource() {
		result.binary = true
		return result, nil
	}
	text, binary, err := readText(file, limit)
	if err != nil {
		return indexedFile{}, err
	}
	result.binary = binary
	if !binary {
		result.source = text
	}
	return result, nil
}

// readText sniffs r's prefix, then reads its text up to limit bytes in total
// (when limit >= 0). A binary file stops at the prefix.
func readText(r io.Reader, limit int64) (text []byte, binary bool, err error) {
	prefix, err := io.ReadAll(io.LimitReader(r, document.SniffLength))
	if err != nil {
		return nil, false, err
	}
	if document.IsBinary(prefix) {
		return prefix, true, nil
	}
	if limit >= 0 {
		r = io.LimitReader(r, max(limit-int64(len(prefix)), 0))
	}
	rest, err := io.ReadAll(r)
	if err != nil {
		return nil, false, err
	}
	return append(prefix, rest...), false, nil
}

// readCommitDocument reads name from the commit at, a full hash, under the
// same limits as readIndexed. It returns the status and message for a
// failure.
func (a *App) readCommitDocument(root, at, name string, fileType document.Type, limit int64) (indexedFile, int, string) {
	hash, ok := git.ParseHash(at)
	if !ok {
		return indexedFile{}, http.StatusBadRequest, invalidCommitMessage
	}
	repo, err := git.Open(root)
	if errors.Is(err, git.ErrNotRepository) {
		return indexedFile{}, http.StatusNotFound, commitUnavailableMessage
	}
	if err != nil {
		return indexedFile{}, http.StatusInternalServerError, "Couldn't read git history: " + err.Error()
	}
	defer func() { _ = repo.Close() }()
	blob, err := repo.CommitBlob(hash, name)
	if errors.Is(err, git.ErrUnavailable) {
		return indexedFile{}, http.StatusNotFound, commitFileUnavailableMessage
	}
	if err != nil {
		return indexedFile{}, http.StatusInternalServerError, fmt.Sprintf("Could not read %s: %v", name, err)
	}
	file := indexedFile{size: blob.Size}
	if limit >= 0 && file.size > limit {
		file.tooLarge = true
		return file, http.StatusOK, ""
	}
	if !fileType.HasTextSource() {
		file.binary = true
		return file, http.StatusOK, ""
	}
	reader, err := blob.Open()
	if err != nil {
		return indexedFile{}, http.StatusInternalServerError, fmt.Sprintf("Could not read %s: %v", name, err)
	}
	defer func() { _ = reader.Close() }()
	text, binary, err := readText(reader, limit)
	if err != nil {
		return indexedFile{}, http.StatusInternalServerError, fmt.Sprintf("Could not read %s: %v", name, err)
	}
	file.binary = binary
	if !binary {
		file.source = text
	}
	return file, http.StatusOK, ""
}

func (a *App) serveImage(w http.ResponseWriter, projectID, name string) {
	contentType, ok := a.config.Types.MediaType(name)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	project, status, _ := a.project(projectID)
	if status != http.StatusOK {
		http.NotFound(w, nil)
		return
	}
	resolved, err := files.ResolveFile(project.root, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}

func (a *App) serveSearchDocuments(w http.ResponseWriter, projectID string) {

	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	index, err := a.scan(project.root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	response := searchDocumentsResponse{
		Documents: make([]searchDocument, 0, len(index.Files)),
		Warnings:  warningStrings(index.Warnings),
	}
	for _, indexedPath := range index.Files {
		if index.Ignored(indexedPath) {
			continue
		}
		resolved, err := index.Resolve(indexedPath)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		family := a.config.Types.Lookup(indexedPath)
		file, err := a.readIndexed(resolved, family.Type(), document.SearchLimit)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		if file.tooLarge || file.binary {
			continue
		}
		// Search reads original text, as VS Code does, so matches map to source lines.
		response.Documents = append(response.Documents, searchDocument{
			Path:    indexedPath,
			Name:    path.Base(indexedPath),
			Content: string(file.source),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

// serveFormat returns the on-demand formatted view of a formattable file.
func (a *App) serveFormat(w http.ResponseWriter, projectID, name, at string) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	family := a.config.Types.Lookup(name)
	fileType := family.Type()
	var file indexedFile
	if at != "" {
		if !fileType.Formattable {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fileType.Label + " files cannot be formatted."})
			return
		}
		var message string
		file, status, message = a.readCommitDocument(project.root, at, name, fileType, document.PreviewLimit)
		if status != http.StatusOK {
			writeJSON(w, status, errorResponse{Error: message})
			return
		}
	} else {
		index, err := a.scan(project.root)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		resolved, err := index.Resolve(name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "That file is unavailable."})
			return
		}
		if !fileType.Formattable {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fileType.Label + " files cannot be formatted."})
			return
		}
		file, err = a.readIndexed(resolved, fileType, document.PreviewLimit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
	}
	if file.tooLarge {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "File is too large to format."})
		return
	}
	if file.binary {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "binary files cannot be formatted"})
		return
	}
	formatted, err := family.Format(file.source)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, formatResponse{Formatted: string(formatted)})
}

// serveRaw returns the bytes of an indexed image-preview file in a sandbox.
func (a *App) serveRaw(w http.ResponseWriter, projectID, name string) {
	project, status, _ := a.project(projectID)
	if status != http.StatusOK {
		http.NotFound(w, nil)
		return
	}
	index, err := a.scan(project.root)
	if err != nil {
		http.Error(w, "could not scan files", http.StatusInternalServerError)
		return
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	contentType, ok := a.config.Types.MediaType(name)
	if !ok {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	file, err := a.openFile(resolved)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, file)
}

func (a *App) servePageData(w http.ResponseWriter, projectID, selected, at string, required bool) {
	data, status := a.page(projectID, selected, at, required)
	writeJSON(w, status, data)
}

// page reads a project's tree and, when selected is set, that document: from
// the working tree, or with at (a full commit hash) from that commit.
func (a *App) page(projectID, selected, at string, required bool) (pageData, int) {
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		return pageData{
			Roots:              a.roots,
			Project:            projectID,
			ProjectUnavailable: true,
			ProjectKind:        projects.KindFolder,
			Selected:           selected,
			Tree:               []treeNode{},
			FilenamePatterns:   a.config.Filenames.Patterns(),
			Error:              message,
			Warnings:           []string{},
		}, status
	}
	checkout := projects.Inspect(project.root)
	index, err := a.scan(project.root)
	if err != nil {
		return pageData{
			Roots:            a.roots,
			Project:          project.id,
			RootName:         filepath.Base(project.root),
			Repository:       checkout.Repository,
			ProjectKind:      checkout.Kind,
			Branch:           checkout.Branch,
			Worktrees:        checkout.Worktrees,
			Tree:             []treeNode{},
			FilenamePatterns: a.config.Filenames.Patterns(),
			Error:            err.Error(),
			Warnings:         []string{},
		}, http.StatusInternalServerError
	}
	data := pageData{
		Roots:            a.roots,
		Project:          project.id,
		RootName:         filepath.Base(index.RootPath()),
		Repository:       checkout.Repository,
		ProjectKind:      checkout.Kind,
		Branch:           checkout.Branch,
		Worktrees:        checkout.Worktrees,
		Selected:         selected,
		FileCount:        len(index.Files),
		Empty:            len(index.Files) == 0,
		Warnings:         warningStrings(index.Warnings),
		FilenamePatterns: a.config.Filenames.Patterns(),
	}
	// A commit document selects no working-tree file.
	treeSelected := selected
	if at != "" {
		treeSelected = ""
	}
	data.Tree = a.makeTree(index.Tree.Children, treeSelected)
	if (required || at != "") && selected == "" {
		data.Error = "No file was selected."
		return data, http.StatusBadRequest
	}

	if selected != "" {
		family := a.config.Types.Lookup(selected)
		fileType := family.Type()
		var file indexedFile
		if at != "" {
			// A commit document is read from git, repository-scoped as
			// history is; it has no preview content and no code navigation.
			data.At = at
			var status int
			file, status, data.Error = a.readCommitDocument(project.root, at, selected, fileType, document.DisplayReadLimit(fileType))
			if status != http.StatusOK {
				return data, status
			}
		} else {
			resolved, err := index.Resolve(selected)
			if err != nil {
				status := http.StatusNotFound
				if errors.Is(err, files.ErrPathEscape) {
					status = http.StatusBadRequest
				}
				data.Error = "That file is unavailable. Refresh the index and choose another file."
				return data, status
			}
			file, err = a.readIndexed(resolved, fileType, document.DisplayReadLimit(fileType))
			if err != nil {
				data.Error = fmt.Sprintf("Could not read %s: %v", selected, err)
				return data, http.StatusInternalServerError
			}
			_, data.NavigationSupported = a.servers.ServerFor(fileType.ID)
		}
		data.DocumentType = &fileType
		data.FileSize = file.size
		data.HasFile = true
		data.Binary = file.binary
		available := document.Available(fileType, file.size, file.binary)
		data.TooLarge = available.TooLarge
		data.PreviewDisabled = available.PreviewDisabled
		data.HighlightDisabled = available.HighlightDisabled
		data.SearchExcluded = available.SearchExcluded
		if file.tooLarge || file.binary {
			return data, http.StatusOK
		}
		data.Source = string(file.source)
		if file.size > document.PreviewLimit || at != "" {
			return data, http.StatusOK
		}
		var err error
		data.Content, err = family.Preview(file.source, document.Context{Path: selected, Project: project.id, Supports: a.config.Filenames.Match})
		if err != nil {
			data.PreviewError = err.Error()
		}
	}
	return data, http.StatusOK
}

func (a *App) makeTree(entries []files.Entry, selected string) []treeNode {
	nodes := make([]treeNode, 0, len(entries))
	for _, entry := range entries {
		node := treeNode{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Selected: entry.Path == selected}
		node.Children = a.makeTree(entry.Children, selected)
		if !entry.IsDir {
			node.FileType = a.config.Types.Lookup(entry.Path).Type().ID
		}
		node.Open = entry.IsDir && (selected == entry.Path || strings.HasPrefix(selected, entry.Path+"/"))
		nodes = append(nodes, node)
	}
	return nodes
}
