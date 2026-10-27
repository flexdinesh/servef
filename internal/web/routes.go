package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/flexdinesh/servef/internal/walkthrough"
)

// contentSecurityPolicy lets pages load only servef's own scripts, styles,
// fonts, and workers.
const contentSecurityPolicy = "default-src 'none'; img-src data: blob: http: https:; font-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'"

// ServeHTTP refuses requests for other hosts, then routes. Every request
// rescans the project it names.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !localHost(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	a.routes.ServeHTTP(w, r)
}

// localHost reports whether a request's Host names this machine directly:
// localhost or an IP literal. A site that rebinds its own hostname to this
// address is refused, so other origins cannot read served files.
func localHost(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil
}

// apiHandler serves one JSON endpoint from its query.
type apiHandler func(w http.ResponseWriter, r *http.Request, query url.Values)

// newRoutes lists every endpoint. All are GET but publishing and deleting
// walkthroughs; JSON endpoints are never cached.
func (a *App) newRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	api := func(path string, handler apiHandler) {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			handler(w, r, r.URL.Query())
		})
	}

	mux.HandleFunc("GET /{$}", a.serveShellPage)
	mux.HandleFunc("GET /view", a.serveShellPage)
	mux.HandleFunc("GET /assets/{name...}", func(w http.ResponseWriter, r *http.Request) {
		a.serveFrontendAsset(w, r.PathValue("name"))
	})
	for path, asset := range noticeAssets {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) { a.serveAsset(w, asset) })
	}

	// Projects.
	api("/api/projects", func(w http.ResponseWriter, _ *http.Request, _ url.Values) { a.serveProjects(w) })
	api("/api/directories", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveDirectories(w, query.Get("path"))
	})

	// Documents and search.
	api("/api/page", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		selected, required := query["path"]
		name := ""
		if len(selected) > 0 {
			name = selected[0]
		}
		a.servePageData(w, query.Get("project"), name, query.Get("at"), required)
	})
	api("/api/format", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveFormat(w, query.Get("project"), query.Get("path"), query.Get("at"))
	})
	api("/api/search-documents", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveSearchDocuments(w, query.Get("project"))
	})
	mux.HandleFunc("GET /api/raw", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		a.serveRaw(w, query.Get("project"), query.Get("path"))
	})
	mux.HandleFunc("GET /api/image", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		a.serveImage(w, query.Get("project"), query.Get("path"))
	})

	// Code navigation.
	api("/api/navigation-status", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveNavigationStatus(w, r, query.Get("project"), query.Get("path"), query.Get("refresh") == "1")
	})
	for path, references := range map[string]bool{"/api/definition": false, "/api/references": true} {
		api(path, func(w http.ResponseWriter, r *http.Request, query url.Values) {
			a.serveNavigation(w, r, references, query.Get("project"), query.Get("path"), query.Get("line"), query.Get("column"))
		})
	}

	// Source Control.
	api("/api/changes", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveChanges(w, r, query.Get("project"))
	})
	api("/api/diff", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveDiffs(w, r, query.Get("project"), query)
	})
	api("/api/commits", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveCommits(w, r, query.Get("project"), query)
	})
	api("/api/commit-files", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveCommitFiles(w, r, query.Get("project"), query.Get("commit"))
	})
	api("/api/branch-files", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveBranchFiles(w, r, query.Get("project"), query)
	})
	api("/api/branch-stack", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveBranchStack(w, r, query.Get("project"), query.Get("base"))
	})
	api("/api/range-files", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveRangeFiles(w, r, query.Get("project"), query.Get("from"), query.Get("to"))
	})
	api("/api/refs", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveRefs(w, query.Get("project"))
	})
	api("/api/compare-stats", func(w http.ResponseWriter, r *http.Request, query url.Values) {
		a.serveCompareStats(w, r, query.Get("project"), query)
	})

	// Walkthroughs (decision 0047).
	api("/api/walkthroughs", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveWalkthroughs(w, query.Get("project"))
	})
	api("/api/walkthrough", func(w http.ResponseWriter, _ *http.Request, query url.Values) {
		a.serveWalkthrough(w, query.Get("project"), query.Get("id"))
	})
	mux.HandleFunc("GET /api/walkthrough-schema", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(walkthrough.Schema)
	})
	mux.HandleFunc("POST /api/walkthroughs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		a.servePublishWalkthrough(w, r)
	})
	mux.HandleFunc("DELETE /api/walkthroughs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		query := r.URL.Query()
		a.serveDeleteWalkthrough(w, r, query.Get("project"), query.Get("id"))
	})

	// Live updates (decision 0050).
	mux.HandleFunc("GET /api/events", a.serveEvents)

	api("/api/metrics", func(w http.ResponseWriter, _ *http.Request, _ url.Values) { a.serveMetrics(w) })
	return mux
}

// serveShellPage serves the app shell. Without a project the browser
// chooses one, so no page data is embedded; /view requires a file unless it
// opens a review, walkthrough, or Welcome tab.
func (a *App) serveShellPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project")
	if projectID == "" {
		a.serveShell(w, http.StatusOK, nil)
		return
	}
	selected, at := "", ""
	required := r.URL.Path == "/view" && !reviewQuery(query) && !walkthroughQuery(query) && !welcomeQuery(query)
	if required {
		selected, at = query.Get("path"), query.Get("at")
	}
	data, status := a.page(projectID, selected, at, required)
	a.serveShell(w, status, &data)
}

// reviewQuery reports a pathless /view that opens a review tab (decision
// 0035): review=working|branch, commit=<hash>, or from=<hash>&to=<hash>.
func reviewQuery(query url.Values) bool {
	return !query.Has("path") && (query.Has("review") || query.Has("commit") || query.Has("from") && query.Has("to"))
}

// walkthroughQuery reports a pathless /view that opens a walkthrough's cover
// tab (decision 0047): wt=<id>.
func walkthroughQuery(query url.Values) bool {
	return !query.Has("path") && query.Get("wt") != ""
}

// welcomeQuery reports a pathless /view that opens the Welcome tab (decision
// 0052): welcome=1.
func welcomeQuery(query url.Values) bool {
	return !query.Has("path") && query.Get("welcome") == "1"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
