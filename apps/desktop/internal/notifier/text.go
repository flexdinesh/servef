package notifier

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// Limits on text a server supplies, in characters.
const (
	maxName = 64
	maxText = 120
)

// clean makes server-supplied text safe to show: a remote server is not trusted
// (decision 0099). It drops control and format characters, which include
// newlines and bidirectional overrides that could disguise a notice, and bounds
// the length.
func clean(text string, limit int) string {
	var out strings.Builder
	count := 0
	for _, r := range text {
		if r == unicode.ReplacementChar || unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if count == limit {
			return strings.TrimSpace(out.String()) + "…"
		}
		out.WriteRune(r)
		count++
	}
	return strings.TrimSpace(out.String())
}

var linuxBody = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// BodyFor is a notice's body as goos shows it. Linux notification servers may
// render a body's markup, and mygo passes it to them as it is, so a commit
// subject must not reach them as markup. Other platforms show plain text.
func BodyFor(goos, body string) string {
	if goos == "linux" {
		return linuxBody.Replace(body)
	}
	return body
}

// noticeID is stable for a connection, project, and kind, so a newer notice
// replaces the one still shown. It holds no secret, and no path a notification
// log would keep.
func noticeID(connection, project string, kind Kind) string {
	sum := sha256.Sum256([]byte(project))
	return connection + "/" + hex.EncodeToString(sum[:8]) + "/" + string(kind)
}
