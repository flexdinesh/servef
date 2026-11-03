package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/git"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Commit document messages.
const (
	CommitUnavailableMessage = "That commit is unavailable."
	InvalidCommitMessage     = "That commit hash is invalid."
	// CommitFileUnavailableMessage reports an unknown commit, or a path the
	// commit does not have.
	CommitFileUnavailableMessage = "That file is unavailable at that commit."
)

// Documents reads project trees, documents, formatted views, search text,
// and raw image bytes. Every operation rescans the project it names.
type Documents struct{ *state }

// indexedFile is a read of an indexed file. Binary files keep only the
// sniffed prefix; files over the read limit are not read.
type indexedFile struct {
	source   []byte
	size     int64
	binary   bool
	tooLarge bool
}

// RawFile is an open file and the media type it is served as. The caller
// closes File.
type RawFile struct {
	File      fs.File
	MediaType string
}

// readIndexed reads a resolved file unless it is larger than limit (when
// limit >= 0). Binary files stop after the sniffed prefix; types without a
// text source are binary without reading content.
func (d *Documents) readIndexed(resolved string, fileType document.Type, limit int64) (indexedFile, error) {
	file, err := d.config.OpenFile(resolved)
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
	text, binary, err := document.ReadText(file, limit)
	if err != nil {
		return indexedFile{}, err
	}
	result.binary = binary
	if !binary {
		result.source = text
	}
	return result, nil
}

// readCommitDocument reads name from the commit at, a full hash, under the
// same limits as readIndexed.
func readCommitDocument(root, at, name string, fileType document.Type, limit int64) (indexedFile, error) {
	hash, ok := git.ParseHash(at)
	if !ok {
		return indexedFile{}, fail(contract.KindInvalid, InvalidCommitMessage)
	}
	repo, err := git.Open(root)
	if errors.Is(err, git.ErrNotRepository) {
		return indexedFile{}, fail(contract.KindNotFound, CommitUnavailableMessage)
	}
	if err != nil {
		return indexedFile{}, fail(contract.KindInternal, "Couldn't read git history: "+err.Error())
	}
	defer func() { _ = repo.Close() }()
	blob, err := repo.CommitBlob(hash, name)
	if errors.Is(err, git.ErrUnavailable) {
		return indexedFile{}, fail(contract.KindNotFound, CommitFileUnavailableMessage)
	}
	if err != nil {
		return indexedFile{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s: %v", name, err))
	}
	file := indexedFile{size: blob.Size}
	if limit >= 0 && file.size > limit {
		file.tooLarge = true
		return file, nil
	}
	if !fileType.HasTextSource() {
		file.binary = true
		return file, nil
	}
	reader, err := blob.Open()
	if err != nil {
		return indexedFile{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s: %v", name, err))
	}
	defer func() { _ = reader.Close() }()
	text, binary, err := document.ReadText(reader, limit)
	if err != nil {
		return indexedFile{}, fail(contract.KindInternal, fmt.Sprintf("Could not read %s: %v", name, err))
	}
	file.binary = binary
	if !binary {
		file.source = text
	}
	return file, nil
}

// OpenImage opens a Markdown image dependency: any file of an image media
// type within the project, indexed or not (root-confined). Every failure is
// not found.
func (d *Documents) OpenImage(projectID, name string) (RawFile, error) {
	notFound := fail(contract.KindNotFound, "That image is unavailable.")
	mediaType, ok := d.config.Types.MediaType(name)
	if !ok {
		return RawFile{}, notFound
	}
	project, err := d.projectWorkspace(projectID)
	if err != nil {
		return RawFile{}, notFound
	}
	resolved, err := files.ResolveFile(project.Root, name)
	if err != nil {
		return RawFile{}, notFound
	}
	file, err := d.config.OpenFile(resolved)
	if err != nil {
		return RawFile{}, notFound
	}
	return RawFile{File: file, MediaType: mediaType}, nil
}

// OpenRaw opens an indexed image-preview file. A missing project or file is
// not found; a type with no media type is unsupported.
func (d *Documents) OpenRaw(projectID, name string) (RawFile, error) {
	notFound := fail(contract.KindNotFound, "That file is unavailable.")
	project, err := d.projectWorkspace(projectID)
	if err != nil {
		return RawFile{}, notFound
	}
	index, err := d.scan(project.Root)
	if err != nil {
		return RawFile{}, fail(contract.KindInternal, "could not scan files")
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		return RawFile{}, notFound
	}
	mediaType, ok := d.config.Types.MediaType(name)
	if !ok {
		return RawFile{}, fail(contract.KindUnsupported, "unsupported media type")
	}
	file, err := d.config.OpenFile(resolved)
	if err != nil {
		return RawFile{}, notFound
	}
	return RawFile{File: file, MediaType: mediaType}, nil
}

// SearchDocuments returns the original text of every non-ignored, non-binary
// file within the search limit; unreadable files become warnings.
func (d *Documents) SearchDocuments(projectID string) (contract.SearchDocumentsResponse, error) {
	project, err := d.projectWorkspace(projectID)
	if err != nil {
		return contract.SearchDocumentsResponse{}, err
	}
	index, err := d.scan(project.Root)
	if err != nil {
		return contract.SearchDocumentsResponse{}, fail(contract.KindInternal, err.Error())
	}
	response := contract.SearchDocumentsResponse{
		Documents: make([]contract.SearchDocument, 0, len(index.Files)),
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
		family := d.config.Types.Lookup(indexedPath)
		file, err := d.readIndexed(resolved, family.Type(), document.SearchLimit)
		if err != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("%s: %v", indexedPath, err))
			continue
		}
		if file.tooLarge || file.binary {
			continue
		}
		// Search reads original text, as VS Code does, so matches map to source lines.
		response.Documents = append(response.Documents, contract.SearchDocument{
			Path:    indexedPath,
			Name:    path.Base(indexedPath),
			Content: string(file.source),
		})
	}
	return response, nil
}

// Format returns the on-demand formatted view of a formattable file, from
// the working tree or a commit.
func (d *Documents) Format(request contract.FormatRequest) (contract.FormatResult, error) {
	project, err := d.projectWorkspace(request.Project)
	if err != nil {
		return contract.FormatResult{}, err
	}
	family := d.config.Types.Lookup(request.Path)
	fileType := family.Type()
	unformattable := fail(contract.KindInvalid, fileType.Label+" files cannot be formatted.")
	var file indexedFile
	if request.At != "" {
		if !fileType.Formattable {
			return contract.FormatResult{}, unformattable
		}
		file, err = readCommitDocument(project.Root, request.At, request.Path, fileType, document.PreviewLimit)
		if err != nil {
			return contract.FormatResult{}, err
		}
	} else {
		index, err := d.scan(project.Root)
		if err != nil {
			return contract.FormatResult{}, fail(contract.KindInternal, err.Error())
		}
		resolved, err := index.Resolve(request.Path)
		if err != nil {
			return contract.FormatResult{}, fail(contract.KindNotFound, "That file is unavailable.")
		}
		if !fileType.Formattable {
			return contract.FormatResult{}, unformattable
		}
		file, err = d.readIndexed(resolved, fileType, document.PreviewLimit)
		if err != nil {
			return contract.FormatResult{}, fail(contract.KindInternal, err.Error())
		}
	}
	if file.tooLarge {
		return contract.FormatResult{}, fail(contract.KindTooLarge, "File is too large to format.")
	}
	if file.binary {
		return contract.FormatResult{}, fail(contract.KindUnprocessable, "binary files cannot be formatted")
	}
	formatted, err := family.Format(file.source)
	if err != nil {
		return contract.FormatResult{}, fail(contract.KindUnprocessable, err.Error())
	}
	return contract.FormatResult{Formatted: string(formatted)}, nil
}

// Page reads a project's tree and, when request.Path is set, that document:
// from the working tree, or with At (a full commit hash) from that commit.
// It always returns a page; a failure also returns the *Error whose message
// the page carries.
func (d *Documents) Page(request contract.PageRequest) (contract.PageData, error) {
	roots := d.config.Roots.List()
	selected, at := request.Path, request.At
	project, err := ResolveProject(roots, request.Project)
	if err != nil {
		failure := AsError(err)
		return contract.PageData{
			Roots:              roots,
			SessionKey:         d.sessionKey(roots),
			Project:            request.Project,
			ProjectUnavailable: true,
			ProjectKind:        contract.ProjectFolder,
			Selected:           selected,
			Tree:               []contract.TreeNode{},
			Error:              failure.Message,
			Warnings:           []string{},
		}, failure
	}
	checkout := projects.Inspect(project.Root)
	index, err := d.scan(project.Root)
	if err != nil {
		return contract.PageData{
			Roots:       roots,
			SessionKey:  d.sessionKey(roots),
			Project:     project.ID,
			RootName:    filepath.Base(project.Root),
			Repository:  checkout.Repository,
			ProjectKind: contract.ProjectKind(checkout.Kind),
			Branch:      checkout.Branch,
			Worktrees:   checkout.Worktrees,
			Tree:        []contract.TreeNode{},
			Error:       err.Error(),
			Warnings:    []string{},
		}, fail(contract.KindInternal, err.Error())
	}
	data := contract.PageData{
		Roots:       roots,
		SessionKey:  d.sessionKey(roots),
		Project:     project.ID,
		RootName:    filepath.Base(index.RootPath()),
		Repository:  checkout.Repository,
		ProjectKind: contract.ProjectKind(checkout.Kind),
		Branch:      checkout.Branch,
		Worktrees:   checkout.Worktrees,
		Selected:    selected,
		FileCount:   len(index.Files),
		Empty:       len(index.Files) == 0,
		Warnings:    warningStrings(index.Warnings),
	}
	// A commit document selects no working-tree file.
	treeSelected := selected
	if at != "" {
		treeSelected = ""
	}
	data.Tree = d.makeTree(index.Tree.Children, treeSelected)
	if (request.Required || at != "") && selected == "" {
		data.Error = "No file was selected."
		return data, fail(contract.KindInvalid, data.Error)
	}
	if selected == "" {
		return data, nil
	}

	family := d.config.Types.Lookup(selected)
	fileType := family.Type()
	var file indexedFile
	if at != "" {
		// A commit document is read from git, repository-scoped as history
		// is; it has no preview content and no code navigation.
		data.At = at
		file, err = readCommitDocument(project.Root, at, selected, fileType, document.DisplayReadLimit(fileType))
		if err != nil {
			failure := AsError(err)
			data.Error = failure.Message
			return data, failure
		}
	} else {
		resolved, err := index.Resolve(selected)
		if err != nil {
			kind := contract.KindNotFound
			if errors.Is(err, files.ErrPathEscape) {
				kind = contract.KindInvalid
			}
			data.Error = "That file is unavailable. Refresh the index and choose another file."
			return data, fail(kind, data.Error)
		}
		file, err = d.readIndexed(resolved, fileType, document.DisplayReadLimit(fileType))
		if err != nil {
			data.Error = fmt.Sprintf("Could not read %s: %v", selected, err)
			return data, fail(contract.KindInternal, data.Error)
		}
		data.NavigationSupported = d.config.Navigation(fileType.ID)
	}
	documentType := fileTypeContract(fileType)
	data.DocumentType = &documentType
	data.FileSize = file.size
	data.HasFile = true
	data.Binary = file.binary
	available := document.Available(fileType, file.size, file.binary)
	data.TooLarge = available.TooLarge
	data.PreviewDisabled = available.PreviewDisabled
	data.HighlightDisabled = available.HighlightDisabled
	data.SearchExcluded = available.SearchExcluded
	if file.tooLarge || file.binary {
		return data, nil
	}
	data.Source = string(file.source)
	if file.size > document.PreviewLimit || at != "" {
		return data, nil
	}
	data.Content, err = family.Preview(file.source, document.Context{Path: selected, Project: project.ID})
	if err != nil {
		data.PreviewError = err.Error()
	}
	return data, nil
}

func (d *Documents) projectWorkspace(id string) (Workspace, error) {
	return ResolveProject(d.config.Roots.List(), id)
}

func (d *Documents) makeTree(entries []files.Entry, selected string) []contract.TreeNode {
	nodes := make([]contract.TreeNode, 0, len(entries))
	for _, entry := range entries {
		node := contract.TreeNode{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Selected: entry.Path == selected}
		node.Children = d.makeTree(entry.Children, selected)
		if !entry.IsDir {
			node.FileType = d.config.Types.Lookup(entry.Path).Type().ID
		}
		node.Open = entry.IsDir && (selected == entry.Path || strings.HasPrefix(selected, entry.Path+"/"))
		nodes = append(nodes, node)
	}
	return nodes
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
