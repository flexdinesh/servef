package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
)

type projectsResponse struct {
	// SessionKey keys the browser's last-project memory (App.sessionKey).
	SessionKey string             `json:"sessionKey"`
	Roots      []projects.Root    `json:"roots"`
	Projects   []projects.Project `json:"projects"`
	Warnings   []string           `json:"warnings"`
}

type directoriesResponse struct {
	Path        string               `json:"path"`
	Directories []projects.Directory `json:"directories"`
}

func (a *App) serveProjects(w http.ResponseWriter) {
	roots := a.roots.List()
	// A root removed while serving is a warning, not a failure for every root.
	discovery := projects.Discover(roots, a.config.ScanDepth)
	writeJSON(w, http.StatusOK, projectsResponse{
		SessionKey: a.sessionKey(roots),
		Roots:      projects.Roots(roots),
		Projects:   discovery.Projects,
		Warnings:   warningStrings(append(discovery.Skipped, discovery.Warnings...)),
	})
}

// sessionKey names the session whose browser memory a page reads:
// Config.SessionKey, else the served roots joined by newlines.
func (a *App) sessionKey(roots []string) string {
	if a.config.SessionKey != "" {
		return a.config.SessionKey
	}
	return strings.Join(roots, "\n")
}

func (a *App) serveDirectories(w http.ResponseWriter, id string) {
	directories, err := projects.Directories(a.roots.List(), id)
	switch {
	case errors.Is(err, files.ErrPathEscape):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "That folder is outside the served folders."})
	case errors.Is(err, projects.ErrInsideRepository):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Repositories open whole; their folders can't be browsed."})
	case errors.Is(err, projects.ErrUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "That folder is unavailable."})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Could not read that folder."})
	default:
		writeJSON(w, http.StatusOK, directoriesResponse{Path: id, Directories: directories})
	}
}
