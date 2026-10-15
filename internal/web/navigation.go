package web

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/navigation"
)

// navigationSource reads a project's navigable files through one scan, taken
// on first use: CLI scope applies, gitignored files are left out, and reads
// stop at the preview limit, past which files are not highlighted or parsed
// either. Create one per request or index build.
type navigationSource struct {
	app   *App
	root  string
	once  sync.Once
	index *files.Index
	err   error
}

func (a *App) navigationSource(root string) *navigationSource {
	return &navigationSource{app: a, root: root}
}

func (s *navigationSource) scan() (*files.Index, error) {
	s.once.Do(func() { s.index, s.err = s.app.scan(s.root) })
	return s.index, s.err
}

func (s *navigationSource) Files() ([]navigation.File, error) {
	index, err := s.scan()
	if err != nil {
		return nil, err
	}
	result := make([]navigation.File, 0, len(index.Files))
	for _, name := range index.Files {
		if index.Ignored(name) {
			continue
		}
		if _, ok := s.app.config.Types.Lookup(name).Navigator(); !ok {
			continue
		}
		resolved, err := index.Resolve(name)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}
		result = append(result, navigation.File{Path: name, Size: info.Size(), Modified: info.ModTime()})
	}
	return result, nil
}

func (s *navigationSource) Read(name string) ([]byte, bool, error) {
	index, err := s.scan()
	if err != nil {
		return nil, false, err
	}
	if index.Ignored(name) {
		return nil, false, nil
	}
	resolved, err := index.Resolve(name)
	if err != nil {
		return nil, false, nil
	}
	file, err := s.app.readIndexed(resolved, s.app.config.Types.Lookup(name).Type(), document.PreviewLimit)
	if err != nil || file.binary || file.tooLarge {
		return nil, false, nil
	}
	return file.source, true, nil
}

// serveNavigation answers /api/definition and /api/references. line is
// 1-based; column counts UTF-16 code units from 0, as the browser does.
func (a *App) serveNavigation(w http.ResponseWriter, r *http.Request, references bool, projectID, name, lineValue, columnValue string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	project, status, message := a.project(projectID)
	if status != http.StatusOK {
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	line, lineErr := strconv.Atoi(lineValue)
	column, columnErr := strconv.Atoi(columnValue)
	if lineErr != nil || columnErr != nil || line < 1 || column < 0 || name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "A file, line, and column are required."})
		return
	}
	source := a.navigationSource(project.root)
	at := document.Position{Line: line, Column: column}
	var result navigation.Result
	var err error
	if references {
		result, err = a.navigation.References(r.Context(), project.root, source, name, at)
	} else {
		result, err = a.navigation.Definition(project.root, source, name, at)
	}
	switch {
	case errors.Is(err, navigation.ErrNotNavigable):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: a.config.Types.Lookup(name).Type().Label + " files do not support code navigation."})
	case errors.Is(err, navigation.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That file is unavailable."})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusOK, result)
	}
}
