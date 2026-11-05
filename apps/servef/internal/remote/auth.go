package remote

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/flexdinesh/servef/pkg/contract"
)

// sessionCookie holds the token in browsers. HttpOnly keeps it from page
// scripts; SameSite=Strict keeps other sites from using it.
const sessionCookie = "servef_remote"

// tokenQuery is the parameter a --web link carries.
const tokenQuery = "token"

// cookieMaxAge is about a month: long enough not to matter, since the token
// itself lives until rotated.
const cookieMaxAge = 30 * 24 * 60 * 60

// Authenticate lets next see only requests with the token: a bearer header from
// the desktop app and CLI, a cookie from browsers. A `?token=` on a page swaps
// for that cookie, and the redirect drops it from the address bar and history.
// It also answers contract.RemoteInfoPath. Every other answer is next's.
//
// Past this point the request is the user's own, so a host name is as good as
// an IP: the Host check guards unauthenticated local servers against DNS
// rebinding, and a rebinding site has neither the token nor the cookie.
func Authenticate(token string, info contract.RemoteInfo, next http.Handler) http.Handler {
	want := []byte(token)
	matches := func(got string) bool { return subtle.ConstantTimeCompare([]byte(got), want) == 1 }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if given, ok := r.URL.Query()[tokenQuery]; ok && r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/view") {
			if len(given) != 1 || !matches(given[0]) {
				refuse(w)
				return
			}
			swapForCookie(w, r, token)
			return
		}
		if !authorized(r, matches) {
			refuse(w)
			return
		}
		if r.URL.Path == contract.RemoteInfoPath {
			writeInfo(w, r, info)
			return
		}
		sameSite(r)
		r.Host = "localhost"
		next.ServeHTTP(w, r)
	})
}

func authorized(r *http.Request, matches func(string) bool) bool {
	if scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
		return matches(value)
	}
	cookie, err := r.Cookie(sessionCookie)
	return err == nil && matches(cookie.Value)
}

// swapForCookie answers with the cookie and the same URL without the token.
func swapForCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	query := r.URL.Query()
	query.Del(tokenQuery)
	target := url.URL{Path: r.URL.Path, RawQuery: query.Encode()}
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

// sameSite removes an Origin that names this request's own host, which the web
// app's write guards would otherwise compare against the rewritten Host. A
// foreign Origin stays, and they refuse it.
func sameSite(r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	if parsed, err := url.Parse(origin); err == nil && parsed.Scheme == "https" && parsed.Path == "" && strings.EqualFold(parsed.Host, r.Host) {
		r.Header.Del("Origin")
	}
}

func writeInfo(w http.ResponseWriter, r *http.Request, info contract.RemoteInfo) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, contract.ErrorResponse{Error: "Method not allowed."})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func refuse(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeJSON(w, http.StatusUnauthorized, contract.ErrorResponse{Error: "This servef server needs a valid connection string.", Code: contract.CodeUnauthorized})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
