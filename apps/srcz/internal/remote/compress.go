package remote

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// minCompress is the smallest known length worth compressing: below it, gzip's
// framing can make the answer larger.
const minCompress = 1024

// gzipWriters reuses compressors: a new one costs over a megabyte. BestSpeed
// because a request's latency matters more here than the last few percent.
var gzipWriters = sync.Pool{New: func() any {
	writer, err := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
	if err != nil {
		panic(err) // BestSpeed is a valid level.
	}
	return writer
}}

// Compress gzips text answers for clients that accept it. A remote link is
// where it pays: search documents and diffs are large and compress well, and
// the local socket costs nothing to send them plain. Event streams, images, and
// anything already encoded pass through.
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &compressWriter{ResponseWriter: w}
		defer writer.finish()
		next.ServeHTTP(writer, r)
	})
}

// acceptsGzip reads Accept-Encoding: gzip, unless its q is zero.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		name, parameters, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		key, value, found := strings.Cut(strings.TrimSpace(parameters), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "q") {
			return true
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return err == nil && q > 0
	}
	return false
}

// compressible reports the media types that shrink: text, JSON, scripts, SVG,
// and WebAssembly. Event streams must reach the client as they are written.
func compressible(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "text/event-stream":
		return false
	case "application/json", "application/javascript", "image/svg+xml", "application/wasm":
		return true
	}
	return strings.HasPrefix(mediaType, "text/")
}

// compressWriter decides at the first write, when the headers are known.
type compressWriter struct {
	http.ResponseWriter
	gzip    *gzip.Writer
	decided bool
}

func (c *compressWriter) decide(status int) {
	if c.decided {
		return
	}
	c.decided = true
	header := c.Header()
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified || header.Get("Content-Encoding") != "" || !compressible(header.Get("Content-Type")) {
		return
	}
	if length, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64); err == nil && length < minCompress {
		return
	}
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	writer, ok := gzipWriters.Get().(*gzip.Writer)
	if !ok {
		return
	}
	writer.Reset(c.ResponseWriter)
	c.gzip = writer
}

// WriteHeader implements http.ResponseWriter.
func (c *compressWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		c.ResponseWriter.WriteHeader(status)
		return
	}
	c.decide(status)
	c.ResponseWriter.WriteHeader(status)
}

// Write implements http.ResponseWriter.
func (c *compressWriter) Write(data []byte) (int, error) {
	c.decide(http.StatusOK)
	if c.gzip != nil {
		return c.gzip.Write(data)
	}
	return c.ResponseWriter.Write(data)
}

// Flush implements http.Flusher.
func (c *compressWriter) Flush() {
	if c.gzip != nil {
		_ = c.gzip.Flush()
	}
	if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection for deadlines.
func (c *compressWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *compressWriter) finish() {
	if c.gzip == nil {
		return
	}
	_ = c.gzip.Close()
	c.gzip.Reset(io.Discard)
	gzipWriters.Put(c.gzip)
	c.gzip = nil
}
