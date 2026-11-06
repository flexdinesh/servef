package web

import (
	"net/http"
)

func (a *App) serveProjects(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, a.app.Projects.List())
}

func (a *App) serveDirectories(w http.ResponseWriter, path string) {
	directories, err := a.app.Projects.Directories(path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, directories)
}
