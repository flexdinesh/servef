package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/flexdinesh/servef/pkg/contract"
)

// languageServersEnabled reports a request's authorization to start
// language servers (contract.LanguageServersHeader: on).
func languageServersEnabled(r *http.Request) bool {
	return r.Header.Get(contract.LanguageServersHeader) == "on"
}

// serveNavigation answers /api/definition and /api/references.
func (a *App) serveNavigation(w http.ResponseWriter, r *http.Request, query url.Values, references bool) {
	request := contract.NavigationRequest{Project: query.Get("project"), Path: query.Get("path"), LanguageServers: languageServersEnabled(r)}
	line, lineErr := strconv.Atoi(query.Get("line"))
	column, columnErr := strconv.Atoi(query.Get("column"))
	// An unparsable position is line 0, which the operation rejects.
	if lineErr == nil && columnErr == nil {
		request.Line, request.Column = line, column
	}
	navigate := a.app.Navigation.Definition
	if references {
		navigate = a.app.Navigation.References
	}
	result, err := navigate(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// serveNavigationStatus answers /api/navigation-status; it never starts a
// language server.
func (a *App) serveNavigationStatus(w http.ResponseWriter, r *http.Request, query url.Values) {
	status, err := a.app.Navigation.Status(contract.NavigationStatusRequest{
		Project:         query.Get("project"),
		Path:            query.Get("path"),
		Refresh:         query.Get("refresh") == "1",
		LanguageServers: languageServersEnabled(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
