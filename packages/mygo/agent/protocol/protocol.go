// Package protocol holds the shapes of mygo's automation protocol
// (plugins/automation, docs/plugins/automation.md): newline-delimited JSON
// requests and answers over a Unix socket. The headless session speaks it,
// and the client reads it from a session or from a built app's
// MYGO_AUTOMATION socket alike.
//
// The headless session adds a few methods a real window has no use for
// (Resize, SetDark, Advance, Shutdown); a built app answers them with
// CodeUnknownMethod.
package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Methods of the protocol.
const (
	MethodWindows = "windows"
	MethodTree    = "tree"
	MethodAct     = "act"
	MethodInput   = "input"
	MethodMenu    = "menu"
	MethodCapture = "capture"
	MethodEval    = "eval"
	MethodIdle    = "idle"

	// Headless session only.
	MethodResize   = "resize"
	MethodSetDark  = "setDark"
	MethodAdvance  = "advance"
	MethodShutdown = "shutdown"
)

// Error codes, as plugins/automation answers them.
const (
	CodeParse         = "parse_error"
	CodeInvalid       = "invalid_request"
	CodeUnknownMethod = "unknown_method"
	CodeNoWindow      = "no_window"
	CodeNoElement     = "no_element"
	CodeUnsupported   = "unsupported"
	CodeTimeout       = "timeout"
	CodeFailed        = "failed"
)

// Request is a line a client sends.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers a request: its Result, or its Error.
type Response struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error is a request's failure.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf returns an Error of code.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Rect is a box in DIPs.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Center returns the middle of r.
func (r Rect) Center() (float64, float64) { return r.X + r.Width/2, r.Y + r.Height/2 }

// Window is an entry of the windows result.
type Window struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	URL           string `json:"url,omitempty"`
	Bounds        Rect   `json:"bounds"`
	ContentBounds Rect   `json:"contentBounds"`
	Focused       bool   `json:"focused"`
	Visible       bool   `json:"visible"`
}

// WindowsResult is the windows result.
type WindowsResult struct {
	Windows []Window `json:"windows"`
}

// WindowParams name a window; 0 is the focused one, else the first.
type WindowParams struct {
	Window int `json:"window,omitempty"`
}

// NodeID is an element's ID: a string of decimal digits on the wire,
// accepted as one or as a number.
type NodeID uint64

// MarshalJSON writes the ID as a string.
func (id NodeID) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, strconv.FormatUint(uint64(id), 10)), nil
}

// UnmarshalJSON reads a string or a number of digits.
func (id *NodeID) UnmarshalJSON(b []byte) error {
	s := string(b)
	if u, err := strconv.Unquote(s); err == nil {
		s = u
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("an element ID is a string of digits, not %s", b)
	}
	*id = NodeID(n)
	return nil
}

func (id NodeID) String() string { return strconv.FormatUint(uint64(id), 10) }

// Node is an element of a tree result.
type Node struct {
	ID          NodeID          `json:"id"`
	Parent      NodeID          `json:"parent,omitempty"`
	Role        string          `json:"role"`
	Key         json.RawMessage `json:"key,omitempty"`
	Label       string          `json:"label,omitempty"`
	Value       string          `json:"value,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Description string          `json:"description,omitempty"`
	Bounds      Rect            `json:"bounds"`
	States      []string        `json:"states,omitempty"`
	Actions     []string        `json:"actions,omitempty"`
	Min         *float64        `json:"min,omitempty"`
	Max         *float64        `json:"max,omitempty"`
	Now         *float64        `json:"now,omitempty"`
	Step        *float64        `json:"step,omitempty"`
	Selection   *[2]int         `json:"selection,omitempty"`
	Level       int             `json:"level,omitempty"`
	PosInSet    int             `json:"posInSet,omitempty"`
	SetSize     int             `json:"setSize,omitempty"`
	WebView     bool            `json:"webView,omitempty"`
}

// Has reports whether n lists action.
func (n Node) Has(action string) bool {
	for _, a := range n.Actions {
		if a == action {
			return true
		}
	}
	return false
}

// TreeResult is the tree result.
type TreeResult struct {
	Window        int      `json:"window"`
	Focus         NodeID   `json:"focus,omitempty"`
	Nodes         []Node   `json:"nodes"`
	Announcements []string `json:"announcements,omitempty"`
}

// ActParams are act's. ID or Key names the element (headless sessions
// have no keys).
type ActParams struct {
	Window int             `json:"window,omitempty"`
	ID     NodeID          `json:"id,omitempty"`
	Key    json.RawMessage `json:"key,omitempty"`
	Action string          `json:"action"`
	Text   string          `json:"text,omitempty"`
	From   int             `json:"from,omitempty"`
	To     int             `json:"to,omitempty"`
}

// InputEvent is an event of an input request.
type InputEvent struct {
	Type    string   `json:"type"`
	X       float64  `json:"x,omitempty"`
	Y       float64  `json:"y,omitempty"`
	Button  string   `json:"button,omitempty"`
	Clicks  int      `json:"clicks,omitempty"`
	DX      float64  `json:"dx,omitempty"`
	DY      float64  `json:"dy,omitempty"`
	Precise bool     `json:"precise,omitempty"`
	Key     string   `json:"key,omitempty"`
	Mods    []string `json:"mods,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// InputParams are input's.
type InputParams struct {
	Window int          `json:"window,omitempty"`
	Events []InputEvent `json:"events"`
}

// MenuParams are menu's.
type MenuParams struct {
	Window int      `json:"window,omitempty"`
	Path   []string `json:"path"`
}

// CaptureResult is the capture result: PNG bytes (base64 on the wire).
type CaptureResult struct {
	PNG    []byte `json:"png"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// EvalParams are eval's.
type EvalParams struct {
	Window int    `json:"window,omitempty"`
	Script string `json:"script"`
}

// IdleParams are idle's; Timeout is in milliseconds, 0 for none.
type IdleParams struct {
	Window  int `json:"window,omitempty"`
	Timeout int `json:"timeout,omitempty"`
}

// ResizeParams are resize's, in DIPs.
type ResizeParams struct {
	Window int `json:"window,omitempty"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DarkParams are setDark's.
type DarkParams struct {
	Window int  `json:"window,omitempty"`
	Dark   bool `json:"dark"`
}

// AdvanceParams are advance's: milliseconds to move the clock on.
type AdvanceParams struct {
	Window int `json:"window,omitempty"`
	MS     int `json:"ms"`
}
