package web

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/flexdinesh/srcz/internal/app"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// Walkthroughs (decision 0047) are published by agents and read by the
// browser through app.Walkthroughs. Publishing is the only write: it accepts
// JSON only, from no foreign origin, so other sites in the browser cannot
// publish or delete.

// sameOrigin reports whether a request came from srcz's own page or from
// outside a browser: a present Origin must name the request's own host.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Path == "" && strings.EqualFold(parsed.Host, r.Host)
}

// writeForeignOrigin refuses a write from another site; it reports whether
// it did.
func writeForeignOrigin(w http.ResponseWriter, r *http.Request) bool {
	if sameOrigin(r) {
		return false
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "Walkthroughs can't be changed from another site.", Code: contract.CodeForeignOrigin})
	return true
}

func (a *App) servePublishWalkthrough(w http.ResponseWriter, r *http.Request) {
	if writeForeignOrigin(w, r) {
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Send the walkthrough as Content-Type: application/json.", Code: contract.CodeUnsupportedMediaType})
		return
	}
	draft, err := app.DecodeWalkthroughDraft(http.MaxBytesReader(w, r.Body, app.MaxWalkthroughBytes))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: fmt.Sprintf("A walkthrough is at most %d bytes.", app.MaxWalkthroughBytes), Code: contract.CodeWalkthroughTooLarge})
		return
	case err != nil:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: contract.CodeInvalidWalkthrough})
		return
	}
	published, err := a.app.Walkthroughs.Publish(r.Context(), draft)
	if err != nil {
		writeError(w, err)
		return
	}
	// The view is absolute at the host the publisher reached.
	published.URL = (&url.URL{Scheme: "http", Host: r.Host}).String() + published.URL
	if draft.Open && a.config.OpenBrowser != nil {
		target := published.URL
		go func() { _ = a.config.OpenBrowser(target) }()
	}
	status := http.StatusOK
	if published.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, published)
}

func (a *App) serveDeleteWalkthrough(w http.ResponseWriter, r *http.Request, projectID, id string) {
	if writeForeignOrigin(w, r) {
		return
	}
	if err := a.app.Walkthroughs.Delete(projectID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
