package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flexdinesh/servef/pkg/contract"
)

// maxResponse bounds a JSON answer; search documents are the largest.
const maxResponse = 512 << 20

// HTTP is the HTTP transport: TCP to a CLI server (NewTCP) or a Unix socket
// to the daemon (NewUnix).
type HTTP struct {
	base   string
	http   *http.Client
	daemon bool
}

var _ Client = (*HTTP)(nil)

// NewTCP returns a client for the CLI server at base, such as
// "http://127.0.0.1:7971". Daemon calls return ErrNoDaemon.
func NewTCP(base string) *HTTP {
	return &HTTP{base: strings.TrimSuffix(base, "/"), http: newHTTPClient(&http.Transport{Proxy: nil})}
}

// NewUnix returns a client for the daemon at socket, sending Host
// "localhost" as the daemon's web routes require.
func NewUnix(socket string) *HTTP {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return &HTTP{base: "http://localhost", http: newHTTPClient(transport), daemon: true}
}

func newHTTPClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (c *HTTP) Projects(ctx context.Context) (contract.ProjectsResponse, error) {
	var response contract.ProjectsResponse
	return response, c.getJSON(ctx, "/api/projects", nil, &response)
}

func (c *HTTP) Directories(ctx context.Context, path string) (contract.DirectoriesResponse, error) {
	var response contract.DirectoriesResponse
	return response, c.getJSON(ctx, "/api/directories", url.Values{"path": {path}}, &response)
}

func (c *HTTP) Page(ctx context.Context, request contract.PageRequest) (contract.PageData, error) {
	query := url.Values{"project": {request.Project}}
	if request.Required || request.Path != "" {
		query.Set("path", request.Path)
	}
	if request.At != "" {
		query.Set("at", request.At)
	}
	response, err := c.do(ctx, http.MethodGet, "/api/page", query, nil)
	if err != nil {
		return contract.PageData{}, err
	}
	defer func() { _ = response.Body.Close() }()
	// A page answers at any status and carries its own error.
	var page contract.PageData
	if err := decode(response.Body, &page); err != nil {
		return contract.PageData{}, fmt.Errorf("servef: page answer (%s): %w", response.Status, err)
	}
	if response.StatusCode != http.StatusOK {
		return page, &Error{Kind: contract.KindForStatus(response.StatusCode), Message: page.Error}
	}
	return page, nil
}

func (c *HTTP) Format(ctx context.Context, request contract.FormatRequest) (contract.FormatResult, error) {
	query := url.Values{"project": {request.Project}, "path": {request.Path}}
	if request.At != "" {
		query.Set("at", request.At)
	}
	var response contract.FormatResult
	return response, c.getJSON(ctx, "/api/format", query, &response)
}

func (c *HTTP) SearchDocuments(ctx context.Context, project string) (contract.SearchDocumentsResponse, error) {
	var response contract.SearchDocumentsResponse
	return response, c.getJSON(ctx, "/api/search-documents", url.Values{"project": {project}}, &response)
}

func (c *HTTP) Image(ctx context.Context, project, path string) (contract.Image, error) {
	response, err := c.do(ctx, http.MethodGet, "/api/image", url.Values{"project": {project}, "path": {path}}, nil)
	if err != nil {
		return contract.Image{}, err
	}
	defer func() { _ = response.Body.Close() }()
	// Image failures are plain text; every not-found image is unavailable.
	switch {
	case response.StatusCode == http.StatusNotFound:
		return contract.Image{}, &Error{Kind: contract.KindNotFound, Message: contract.ImageUnavailableMessage}
	case response.StatusCode != http.StatusOK:
		text, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return contract.Image{}, &Error{Kind: contract.KindForStatus(response.StatusCode), Message: strings.TrimSpace(string(text))}
	}
	tooLarge := &Error{Kind: contract.KindTooLarge, Message: contract.ImageTooLargeMessage}
	if response.ContentLength > contract.MaxImageBytes {
		return contract.Image{}, tooLarge
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, contract.MaxImageBytes+1))
	if err != nil {
		return contract.Image{}, err
	}
	if len(content) > contract.MaxImageBytes {
		return contract.Image{}, tooLarge
	}
	return contract.Image{MediaType: response.Header.Get("Content-Type"), Content: content}, nil
}

func (c *HTTP) Events(ctx context.Context, request contract.EventsRequest) (*Stream[contract.Event], error) {
	query := url.Values{"project": {request.Project}}
	if request.Lease != "" {
		query.Set("lease", request.Lease)
	}
	return openStream(c, ctx, "/api/events", query, projectEvent)
}

func (c *HTTP) Changes(ctx context.Context, project string) (contract.ChangesResponse, error) {
	var response contract.ChangesResponse
	return response, c.getJSON(ctx, "/api/changes", url.Values{"project": {project}}, &response)
}

func (c *HTTP) Diff(ctx context.Context, request contract.DiffRequest) (contract.DiffResponse, error) {
	query := url.Values{"project": {request.Project}, "path": {request.Path}}
	setNonEmpty(query, "oldPath", request.OldPath)
	switch request.Kind {
	case contract.DiffCommit:
		query.Set("commit", request.Commit)
	case contract.DiffBranch:
		query.Set("branch", "1")
	case contract.DiffRange:
		query.Set("from", request.From)
		query.Set("to", request.To)
	}
	// Branch parameters go whatever the kind, so the server rejects them
	// where they do not apply.
	setNonEmpty(query, "base", request.Base)
	setNonEmpty(query, "head", request.Head)
	setFlag(query, "uncommitted", request.Uncommitted)
	setFlag(query, "ignoreWhitespace", request.IgnoreWhitespace)
	var response contract.DiffResponse
	return response, c.getJSON(ctx, "/api/diff", query, &response)
}

func (c *HTTP) Commits(ctx context.Context, request contract.CommitsRequest) (contract.CommitsResponse, error) {
	query := url.Values{"project": {request.Project}}
	setNonEmpty(query, "range", string(request.Range))
	if request.Skip != 0 {
		query.Set("skip", strconv.Itoa(request.Skip))
	}
	if request.Limit != 0 {
		query.Set("limit", strconv.Itoa(request.Limit))
	}
	setNonEmpty(query, "head", request.Head)
	setNonEmpty(query, "base", request.Base)
	var response contract.CommitsResponse
	return response, c.getJSON(ctx, "/api/commits", query, &response)
}

func (c *HTTP) CommitFiles(ctx context.Context, project, commit string) (contract.CommitFilesResponse, error) {
	var response contract.CommitFilesResponse
	return response, c.getJSON(ctx, "/api/commit-files", url.Values{"project": {project}, "commit": {commit}}, &response)
}

func (c *HTTP) BranchFiles(ctx context.Context, request contract.BranchFilesRequest) (contract.BranchFilesResponse, error) {
	query := url.Values{"project": {request.Project}}
	setNonEmpty(query, "base", request.Base)
	setNonEmpty(query, "head", request.Head)
	setFlag(query, "uncommitted", request.Uncommitted)
	var response contract.BranchFilesResponse
	return response, c.getJSON(ctx, "/api/branch-files", query, &response)
}

func (c *HTTP) RangeFiles(ctx context.Context, project, from, to string) (contract.RangeFilesResponse, error) {
	var response contract.RangeFilesResponse
	return response, c.getJSON(ctx, "/api/range-files", url.Values{"project": {project}, "from": {from}, "to": {to}}, &response)
}

func (c *HTTP) Refs(ctx context.Context, project string) (contract.RefsResponse, error) {
	var response contract.RefsResponse
	return response, c.getJSON(ctx, "/api/refs", url.Values{"project": {project}}, &response)
}

func (c *HTTP) BranchStack(ctx context.Context, project, base string) (contract.StackResponse, error) {
	query := url.Values{"project": {project}}
	setNonEmpty(query, "base", base)
	var response contract.StackResponse
	return response, c.getJSON(ctx, "/api/branch-stack", query, &response)
}

func (c *HTTP) CompareStats(ctx context.Context, request contract.CompareStatsRequest) (contract.CompareStatsResponse, error) {
	query := url.Values{"project": {request.Project}, "compare": {string(request.Compare)}}
	setNonEmpty(query, "base", request.Base)
	setNonEmpty(query, "head", request.Head)
	setFlag(query, "uncommitted", request.Uncommitted)
	setNonEmpty(query, "commit", request.Commit)
	setNonEmpty(query, "from", request.From)
	setNonEmpty(query, "to", request.To)
	var response contract.CompareStatsResponse
	return response, c.getJSON(ctx, "/api/compare-stats", query, &response)
}

func (c *HTTP) NavigationStatus(ctx context.Context, request contract.NavigationStatusRequest) (contract.NavigationStatus, error) {
	query := url.Values{"project": {request.Project}, "path": {request.Path}}
	setFlag(query, "refresh", request.Refresh)
	var response contract.NavigationStatus
	return response, c.getJSONWith(ctx, "/api/navigation-status", query, languageServers(request.LanguageServers), &response)
}

func (c *HTTP) Definition(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return c.navigate(ctx, "/api/definition", request)
}

func (c *HTTP) References(ctx context.Context, request contract.NavigationRequest) (contract.NavigationResult, error) {
	return c.navigate(ctx, "/api/references", request)
}

func (c *HTTP) navigate(ctx context.Context, path string, request contract.NavigationRequest) (contract.NavigationResult, error) {
	query := url.Values{
		"project": {request.Project},
		"path":    {request.Path},
		"line":    {strconv.Itoa(request.Line)},
		"column":  {strconv.Itoa(request.Column)},
	}
	var response contract.NavigationResult
	return response, c.getJSONWith(ctx, path, query, languageServers(request.LanguageServers), &response)
}

// languageServers is the header that authorizes language servers, or none.
func languageServers(on bool) http.Header {
	if !on {
		return nil
	}
	return http.Header{contract.LanguageServersHeader: {"on"}}
}

func (c *HTTP) Metrics(ctx context.Context) (contract.ProcessMetrics, error) {
	var response contract.ProcessMetrics
	return response, c.getJSON(ctx, "/api/metrics", nil, &response)
}

// setNonEmpty sets a query parameter the server reads as absent when empty.
func setNonEmpty(query url.Values, name, value string) {
	if value != "" {
		query.Set(name, value)
	}
}

// setFlag sets a parameter whose only value is 1.
func setFlag(query url.Values, name string, on bool) {
	if on {
		query.Set(name, "1")
	}
}

func (c *HTTP) DaemonStatus(ctx context.Context) (contract.DaemonStatus, error) {
	var status contract.DaemonStatus
	if !c.daemon {
		return status, ErrNoDaemon
	}
	return status, c.getJSON(ctx, "/_daemon/status", nil, &status)
}

func (c *HTTP) DaemonRoots(ctx context.Context) (contract.DaemonRoots, error) {
	var roots contract.DaemonRoots
	if !c.daemon {
		return roots, ErrNoDaemon
	}
	return roots, c.getJSON(ctx, "/_daemon/roots", nil, &roots)
}

func (c *HTTP) DaemonOpen(ctx context.Context, path string) (contract.DaemonOpenResult, error) {
	var result contract.DaemonOpenResult
	if !c.daemon {
		return result, ErrNoDaemon
	}
	body, err := json.Marshal(contract.DaemonPathRequest{Path: path})
	if err != nil {
		return result, err
	}
	response, err := c.do(ctx, http.MethodPost, "/_daemon/open", nil, body)
	if err != nil {
		return result, err
	}
	defer func() { _ = response.Body.Close() }()
	return result, answer(response, &result)
}

func (c *HTTP) DaemonDirectories(ctx context.Context, path string) (contract.DaemonDirectories, error) {
	var listing contract.DaemonDirectories
	if !c.daemon {
		return listing, ErrNoDaemon
	}
	return listing, c.getJSON(ctx, "/_daemon/directories", url.Values{"path": {path}}, &listing)
}

func (c *HTTP) DaemonEvents(ctx context.Context) (*Stream[contract.DaemonEvent], error) {
	if !c.daemon {
		return nil, ErrNoDaemon
	}
	return openStream(c, ctx, "/_daemon/events", nil, daemonEvent)
}

// do sends a request; a JSON body is sent as application/json.
func (c *HTTP) do(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Response, error) {
	return c.doWith(ctx, method, path, query, nil, body)
}

// doWith is do with extra request headers.
func (c *HTTP) doWith(ctx context.Context, method, path string, query url.Values, header http.Header, body []byte) (*http.Response, error) {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(request)
}

func (c *HTTP) getJSON(ctx context.Context, path string, query url.Values, result any) error {
	return c.getJSONWith(ctx, path, query, nil, result)
}

func (c *HTTP) getJSONWith(ctx context.Context, path string, query url.Values, header http.Header, result any) error {
	response, err := c.doWith(ctx, http.MethodGet, path, query, header, nil)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	return answer(response, result)
}

// answer decodes a 2xx body into result, or the error body into an *Error.
func answer(response *http.Response, result any) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return decode(response.Body, result)
	}
	return failure(response)
}

func failure(response *http.Response) error {
	var body contract.ErrorResponse
	if err := decode(response.Body, &body); err != nil || body.Error == "" {
		return fmt.Errorf("servef: %s answered %s without an error body", response.Request.URL.Path, response.Status)
	}
	return &Error{Kind: contract.KindForStatus(response.StatusCode), Code: body.Code, Message: body.Error}
}

func decode(body io.Reader, result any) error {
	return json.NewDecoder(io.LimitReader(body, maxResponse)).Decode(result)
}

// openStream opens a server-sent event stream; parse turns each named event
// into T and skips names it does not know.
func openStream[T any](c *HTTP, ctx context.Context, path string, query url.Values, parse func(name string, data []byte) (T, bool, error)) (*Stream[T], error) {
	ctx, cancel := context.WithCancel(ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+encodeQuery(query), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(request)
	if err != nil {
		cancel()
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		defer cancel()
		defer func() { _ = response.Body.Close() }()
		return nil, failure(response)
	}
	reader := bufio.NewReader(response.Body)
	next := func() (T, error) {
		var zero T
		for {
			name, data, err := readEvent(reader)
			if err != nil {
				if ctx.Err() != nil {
					return zero, ctx.Err()
				}
				return zero, err
			}
			event, ok, err := parse(name, data)
			if err != nil {
				return zero, err
			}
			if ok {
				return event, nil
			}
		}
	}
	return &Stream[T]{next: next, close: func() { cancel(); _ = response.Body.Close() }}, nil
}

func encodeQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	return "?" + query.Encode()
}

// readEvent reads one server-sent event, skipping comments; a stream that
// ends between events is io.EOF.
func readEvent(reader *bufio.Reader) (string, []byte, error) {
	var name string
	var data []byte
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", nil, io.EOF
			}
			return "", nil, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch {
		case line == "":
			if name != "" || data != nil {
				return name, data, nil
			}
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimPrefix(strings.TrimPrefix(line, "event:"), " ")
		case strings.HasPrefix(line, "data:"):
			if data != nil {
				data = append(data, '\n')
			}
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")...)
		}
	}
}

func projectEvent(name string, data []byte) (contract.Event, bool, error) {
	event := contract.Event{Name: name}
	var target any
	switch name {
	case contract.EventStatus:
		event.Status = &contract.StatusEvent{}
		target = event.Status
	case contract.EventChange:
		event.Change = &contract.ChangeEvent{}
		target = event.Change
	case contract.EventHead:
		event.Head = &contract.HeadEvent{}
		target = event.Head
	default:
		return contract.Event{}, false, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return contract.Event{}, false, fmt.Errorf("servef: %s event: %w", name, err)
	}
	return event, true, nil
}

func daemonEvent(name string, data []byte) (contract.DaemonEvent, bool, error) {
	event := contract.DaemonEvent{Name: name}
	var target any
	switch name {
	case contract.DaemonEventStatus:
		event.Status = &contract.DaemonStatusEvent{}
		target = event.Status
	case contract.DaemonEventWalkthrough:
		event.Walkthrough = &contract.WalkthroughEvent{}
		target = event.Walkthrough
	default:
		return contract.DaemonEvent{}, false, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return contract.DaemonEvent{}, false, fmt.Errorf("servef: %s event: %w", name, err)
	}
	return event, true, nil
}
