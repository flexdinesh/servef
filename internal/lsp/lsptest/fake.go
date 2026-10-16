// Package lsptest is a fake language server for tests. A test binary runs
// it from TestMain when Env is set, so tests start real server processes
// without installing any.
package lsptest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Env marks a process as the fake server. Other variables shape it:
// SERVEF_FAKE_LSP_LOG names a file that records document notifications,
// SERVEF_FAKE_LSP_SLOW delays initialize, SERVEF_FAKE_LSP_CRASH exits on
// definition requests, and SERVEF_FAKE_LSP_TARGET names the definition
// target relative to the root (default target.go, line 5, column 2).
const Env = "SERVEF_FAKE_LSP"

// Serving reports whether this process should run the fake server.
func Serving() bool { return os.Getenv(Env) == "1" }

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func read(in *bufio.Reader) (message, error) {
	header, err := textproto.NewReader(in).ReadMIMEHeader()
	if err != nil {
		return message{}, err
	}
	length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		return message{}, err
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(in, body); err != nil {
		return message{}, err
	}
	var m message
	return m, json.Unmarshal(body, &m)
}

// Serve answers the protocol on stdio until exit or EOF.
func Serve() {
	in := bufio.NewReader(os.Stdin)
	logFile, _ := os.OpenFile(os.Getenv("SERVEF_FAKE_LSP_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	record := func(format string, args ...any) {
		if logFile != nil {
			_, _ = fmt.Fprintf(logFile, format+"\n", args...)
		}
	}
	send := func(id json.RawMessage, result any) {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		fmt.Printf("Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	target := os.Getenv("SERVEF_FAKE_LSP_TARGET")
	if target == "" {
		target = "target.go"
	}
	var root string
	for {
		request, err := read(in)
		if err != nil {
			return
		}
		switch request.Method {
		case "initialize":
			if delay, err := time.ParseDuration(os.Getenv("SERVEF_FAKE_LSP_SLOW")); err == nil {
				time.Sleep(delay)
			}
			var params struct {
				RootURI string `json:"rootUri"`
			}
			_ = json.Unmarshal(request.Params, &params)
			root = params.RootURI
			send(request.ID, map[string]any{"capabilities": map[string]any{"definitionProvider": true, "referencesProvider": map[string]any{}}})
		case "textDocument/definition":
			if os.Getenv("SERVEF_FAKE_LSP_CRASH") == "1" {
				fmt.Fprintln(os.Stderr, "fake server crashed")
				os.Exit(3)
			}
			send(request.ID, []map[string]any{{
				"targetUri":            root + "/" + filepath.ToSlash(target),
				"targetRange":          map[string]any{"start": map[string]int{"line": 0, "character": 0}},
				"targetSelectionRange": map[string]any{"start": map[string]int{"line": 4, "character": 2}},
			}})
		case "textDocument/references":
			var params struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
				Position struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"position"`
			}
			_ = json.Unmarshal(request.Params, &params)
			send(request.ID, []map[string]any{
				{"uri": params.TextDocument.URI, "range": map[string]any{"start": params.Position}},
				{"uri": "file:///outside/of/the/project.go", "range": map[string]any{"start": params.Position}},
			})
		case "shutdown":
			send(request.ID, nil)
		case "exit":
			return
		case "textDocument/didOpen", "textDocument/didChange":
			var params struct {
				TextDocument struct {
					URI     string `json:"uri"`
					Version int    `json:"version"`
				} `json:"textDocument"`
			}
			_ = json.Unmarshal(request.Params, &params)
			record("%s %s %d", request.Method, filepath.Base(params.TextDocument.URI), params.TextDocument.Version)
		case "workspace/didChangeWatchedFiles":
			var params struct {
				Changes []struct {
					URI  string `json:"uri"`
					Type int    `json:"type"`
				} `json:"changes"`
			}
			_ = json.Unmarshal(request.Params, &params)
			for _, change := range params.Changes {
				record("watched %s %d", filepath.Base(change.URI), change.Type)
			}
		}
	}
}
