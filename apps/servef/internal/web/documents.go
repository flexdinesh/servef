package web

import (
	"errors"
	"io"
	"net/http"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/pkg/contract"
)

// serveImage returns a Markdown image dependency's bytes.
func (a *App) serveImage(w http.ResponseWriter, projectID, name string) {
	image, err := a.app.Documents.OpenImage(projectID, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	defer func() { _ = image.File.Close() }()
	w.Header().Set("Content-Type", image.MediaType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, image.File)
}

func (a *App) serveSearchDocuments(w http.ResponseWriter, projectID string) {
	response, err := a.app.Documents.SearchDocuments(projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// serveFormat returns the on-demand formatted view of a formattable file.
func (a *App) serveFormat(w http.ResponseWriter, projectID, name, at string) {
	formatted, err := a.app.Documents.Format(contract.FormatRequest{Project: projectID, Path: name, At: at})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, formatted)
}

// serveRaw returns the bytes of an indexed image-preview file in a sandbox.
// Its failures are plain text, not JSON.
func (a *App) serveRaw(w http.ResponseWriter, projectID, name string) {
	raw, err := a.app.Documents.OpenRaw(projectID, name)
	if err != nil {
		var failure *app.Error
		if !errors.As(err, &failure) || failure.Kind == contract.KindNotFound {
			http.NotFound(w, nil)
			return
		}
		http.Error(w, failure.Message, contract.HTTPStatus(failure.Kind))
		return
	}
	defer func() { _ = raw.File.Close() }()
	w.Header().Set("Content-Type", raw.MediaType)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.Copy(w, raw.File)
}

// servePageData writes a page at its outcome's status; a failed page still
// carries its project and error.
func (a *App) servePageData(w http.ResponseWriter, projectID, selected, at string, required bool) {
	data, err := a.app.Documents.Page(contract.PageRequest{Project: projectID, Path: selected, At: at, Required: required})
	writeJSON(w, statusOf(err), data)
}
