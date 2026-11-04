package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/packages/mygo/agent/protocol"
)

// maxLine is the longest request line, as plugins/automation takes.
const maxLine = 16 << 20

// Serve answers the connections of l until it closes.
func (s *Session) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.serveConn(c)
	}
}

// serveConn answers a connection's requests in order.
func (s *Session) serveConn(c io.ReadWriteCloser) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	enc := json.NewEncoder(c)
	enc.SetEscapeHTML(false)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		res, shutdown := s.Handle(line)
		if enc.Encode(res) != nil {
			return
		}
		if shutdown {
			s.Close()
			return
		}
	}
	if errors.Is(sc.Err(), bufio.ErrTooLong) {
		_ = enc.Encode(protocol.Response{Error: protocol.Errorf(protocol.CodeParse, "a request is longer than %d bytes", maxLine)})
	}
}

// Handle answers a request line; shutdown reports a shutdown request.
func (s *Session) Handle(line []byte) (res protocol.Response, shutdown bool) {
	var req protocol.Request
	if err := json.Unmarshal(line, &req); err != nil {
		return protocol.Response{Error: protocol.Errorf(protocol.CodeParse, "%v", err)}, false
	}
	res.ID = req.ID
	if req.Method == protocol.MethodShutdown {
		res.Result = json.RawMessage("{}")
		return res, true
	}
	var result any
	var err error
	if doErr := s.Do(func(*ui.Tester) { result, err = s.call(req) }); doErr != nil {
		err = protocol.Errorf(protocol.CodeFailed, "%v", doErr)
	}
	if err != nil {
		var e *protocol.Error
		if !errors.As(err, &e) {
			e = protocol.Errorf(protocol.CodeFailed, "%v", err)
		}
		res.Error = e
		return res, false
	}
	b, err := json.Marshal(result)
	if err != nil {
		res.Error = protocol.Errorf(protocol.CodeFailed, "%v", err)
		return res, false
	}
	res.Result = b
	return res, false
}

type done struct{}

// call performs a request on the session's goroutine.
func (s *Session) call(req protocol.Request) (any, error) {
	window := func(w int) error {
		if w != 0 && w != 1 {
			return protocol.Errorf(protocol.CodeNoWindow, "no window %d", w)
		}
		return nil
	}
	switch req.Method {
	case "":
		return nil, protocol.Errorf(protocol.CodeInvalid, "the request has no method")
	case protocol.MethodWindows:
		if err := params(req, &struct{}{}); err != nil {
			return nil, err
		}
		r := protocol.Rect{Width: float64(s.w), Height: float64(s.h)}
		return protocol.WindowsResult{Windows: []protocol.Window{{ID: 1, Title: s.title(), Kind: "content", Bounds: r, ContentBounds: r, Focused: true, Visible: true}}}, nil
	case protocol.MethodTree:
		var p protocol.WindowParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		return s.tree(), nil
	case protocol.MethodAct:
		var p protocol.ActParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		return done{}, s.act(p)
	case protocol.MethodInput:
		var p protocol.InputParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		return done{}, s.input(p.Events)
	case protocol.MethodMenu:
		var p protocol.MenuParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		if len(p.Path) == 0 {
			return nil, protocol.Errorf(protocol.CodeInvalid, "menu needs the path of the item, as [\"File\", \"New\"]")
		}
		if len(s.tt.Menu()) == 0 {
			return nil, protocol.Errorf(protocol.CodeUnsupported, "a headless window has no menu bar; open a context menu first (right click), then choose from it")
		}
		if err := s.tt.ChooseMenuItem(p.Path...); err != nil {
			return nil, protocol.Errorf(protocol.CodeNoElement, "%v", err)
		}
		s.settle()
		return done{}, nil
	case protocol.MethodCapture:
		var p protocol.WindowParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		img := s.tt.Image()
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return protocol.CaptureResult{PNG: buf.Bytes(), Width: img.Rect.Dx(), Height: img.Rect.Dy()}, nil
	case protocol.MethodEval:
		return nil, protocol.Errorf(protocol.CodeUnsupported, "a headless window shows no page")
	case protocol.MethodIdle:
		var p protocol.IdleParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		if p.Timeout < 0 {
			return nil, protocol.Errorf(protocol.CodeInvalid, "timeout is negative")
		}
		return done{}, s.idle(time.Duration(p.Timeout) * time.Millisecond)
	case protocol.MethodResize:
		var p protocol.ResizeParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		if p.Width <= 0 || p.Height <= 0 {
			return nil, protocol.Errorf(protocol.CodeInvalid, "width and height must be positive")
		}
		s.w, s.h = p.Width, p.Height
		s.tt.SetSize(p.Width, p.Height)
		s.settle()
		return done{}, nil
	case protocol.MethodSetDark:
		var p protocol.DarkParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		s.tt.SetDark(p.Dark)
		s.settle()
		return done{}, nil
	case protocol.MethodAdvance:
		var p protocol.AdvanceParams
		if err := params(req, &p); err != nil {
			return nil, err
		}
		if err := window(p.Window); err != nil {
			return nil, err
		}
		if p.MS < 0 {
			return nil, protocol.Errorf(protocol.CodeInvalid, "ms is negative")
		}
		s.tt.Advance(time.Duration(p.MS) * time.Millisecond)
		return done{}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "no method %q", req.Method)
}

// params decodes a request's params, refusing fields the method does not
// take.
func params(req protocol.Request, p any) error {
	raw := bytes.TrimSpace(req.Params)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(p); err != nil {
		return protocol.Errorf(protocol.CodeInvalid, "params of %s: %v", req.Method, err)
	}
	return nil
}

// act performs an action on the element of the ID, the key, or the label.
// Keys of the headless tree are its labels.
func (s *Session) act(p protocol.ActParams) error {
	key := ""
	if len(bytes.TrimSpace(p.Key)) > 0 {
		if err := json.Unmarshal(p.Key, &key); err != nil {
			return protocol.Errorf(protocol.CodeInvalid, "key: a headless key is a string, the element's text")
		}
	}
	named := 0
	for _, set := range []bool{p.ID != 0, key != "", p.Label != ""} {
		if set {
			named++
		}
	}
	if named != 1 {
		return protocol.Errorf(protocol.CodeInvalid, "act takes one of id, key or label")
	}
	label := p.Label + key
	var target *element
	for _, e := range s.elements() {
		if (p.ID != 0 && e.node.ID == p.ID) || (label != "" && e.node.Label == label && e.occurrence == 0) {
			target = &e
			break
		}
	}
	if target == nil {
		return protocol.Errorf(protocol.CodeNoElement, "no element of the last frame has that id or label")
	}
	if target.menu {
		if p.Action != "press" {
			return protocol.Errorf(protocol.CodeUnsupported, "menu items take press")
		}
		if err := s.tt.ChooseMenuItem(target.node.Label); err != nil {
			return protocol.Errorf(protocol.CodeFailed, "%v", err)
		}
		s.settle()
		return nil
	}
	if target.occurrence > 0 {
		return protocol.Errorf(protocol.CodeUnsupported, "%q shows more than once; the headless tree reaches only its first element: click its position instead", target.node.Label)
	}
	click := func() error {
		if err := s.tt.Click(target.node.Label); err != nil {
			return protocol.Errorf(protocol.CodeNoElement, "%v", err)
		}
		return nil
	}
	switch p.Action {
	case "press", "focus", "expand", "collapse":
		if err := click(); err != nil {
			return err
		}
	case "setValue":
		if err := click(); err != nil {
			return err
		}
		s.tt.Key(ui.Cmd, ui.KeyA)
		if p.Text == "" {
			s.tt.Key(0, ui.KeyBackspace)
		} else {
			s.tt.Type(p.Text)
		}
	case "insertText":
		if err := click(); err != nil {
			return err
		}
		s.tt.Type(p.Text)
	case "increment", "decrement", "scrollIntoView", "selectText":
		return protocol.Errorf(protocol.CodeUnsupported, "a headless window cannot %s; send input instead", p.Action)
	default:
		return protocol.Errorf(protocol.CodeInvalid, "no action %q", p.Action)
	}
	s.settle()
	return nil
}

// input sends events to the tester in order.
func (s *Session) input(events []protocol.InputEvent) error {
	for i, e := range events {
		mods, err := modifiers(e.Mods)
		if err != nil {
			return protocol.Errorf(protocol.CodeInvalid, "event %d: %v", i, err)
		}
		x, y := float32(e.X), float32(e.Y)
		hold := func(do func()) {
			if mods != 0 {
				s.tt.HoldModifiers(mods)
			}
			do()
			if mods != 0 {
				s.tt.HoldModifiers(0)
			}
		}
		switch e.Button {
		case "", "left", "right", "middle":
		default:
			return protocol.Errorf(protocol.CodeInvalid, "event %d: no button %q: left, right or middle", i, e.Button)
		}
		pointer := func() error {
			if e.Button != "" && e.Button != "left" {
				return protocol.Errorf(protocol.CodeUnsupported, "event %d: a headless %s takes the left button; send a right click as a click", i, e.Type)
			}
			return nil
		}
		switch e.Type {
		case "pointerMove":
			hold(func() { s.tt.Move(x, y) })
		case "pointerDown":
			if err := pointer(); err != nil {
				return err
			}
			hold(func() { s.tt.Press(x, y) })
		case "pointerUp":
			if err := pointer(); err != nil {
				return err
			}
			hold(func() { s.tt.Release(x, y) })
		case "click":
			switch e.Button {
			case "right":
				hold(func() { s.tt.RightClickAt(x, y) })
			case "middle":
				return protocol.Errorf(protocol.CodeUnsupported, "event %d: a headless window takes no middle click", i)
			default:
				for range max(e.Clicks, 1) {
					s.tt.ClickAtWith(mods, x, y)
				}
			}
		case "wheel":
			if e.Precise {
				hold(func() { s.tt.Scroll(x, y, float32(e.DX), float32(e.DY)) })
			} else {
				hold(func() { s.tt.Wheel(x, y, float32(e.DX), float32(e.DY)) })
			}
		case "keyDown", "keyUp", "key":
			key, keyMods, err := ParseKey(e.Key)
			if err != nil {
				return protocol.Errorf(protocol.CodeInvalid, "event %d: %v", i, err)
			}
			switch e.Type {
			case "keyDown":
				s.tt.HoldModifiers(keyMods)
				s.tt.KeyDown(key)
			case "keyUp":
				s.tt.KeyUp(key)
				s.tt.HoldModifiers(0)
			default:
				s.tt.Key(keyMods, key)
			}
		case "text":
			if e.Text == "" {
				return protocol.Errorf(protocol.CodeInvalid, "event %d: text has no text", i)
			}
			s.tt.Type(e.Text)
		default:
			return protocol.Errorf(protocol.CodeInvalid, "event %d: no event type %q", i, e.Type)
		}
	}
	s.settle()
	return nil
}

func modifiers(list []string) (ui.Modifiers, error) {
	var mods ui.Modifiers
	for _, name := range list {
		m, ok := modifierNames[strings.ToLower(name)]
		if !ok {
			return 0, errors.New("no modifier " + name)
		}
		mods |= m
	}
	return mods, nil
}

var modifierNames = map[string]ui.Modifiers{
	"shift": ui.Shift, "ctrl": ui.Ctrl, "control": ui.Ctrl, "alt": ui.Alt, "option": ui.Alt,
	"super": ui.Super, "cmd": ui.Super, "command": ui.Super, "meta": ui.Super,
	"cmdorctrl": ui.Cmd, "commandorcontrol": ui.Cmd,
}

var namedKeys = map[string]ui.Key{
	"enter": ui.KeyEnter, "return": ui.KeyEnter, "escape": ui.KeyEscape, "esc": ui.KeyEscape,
	"backspace": ui.KeyBackspace, "tab": ui.KeyTab, "space": ui.KeySpace, "delete": ui.KeyDelete,
	"insert": ui.KeyInsert, "home": ui.KeyHome, "end": ui.KeyEnd, "pageup": ui.KeyPageUp,
	"pagedown": ui.KeyPageDown, "left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp,
	"down": ui.KeyDown, "-": ui.KeyMinus, "=": ui.KeyEqual, ",": ui.KeyComma, ".": ui.KeyPeriod,
	"/": ui.KeySlash, ";": ui.KeySemicolon, "'": ui.KeyQuote, "[": ui.KeyBracketLeft,
	"]": ui.KeyBracketRight, "\\": ui.KeyBackslash, "`": ui.KeyBackquote, "contextmenu": ui.KeyContextMenu,
}

// ParseKey reads a key and its modifiers as menu accelerators write them,
// as "Enter", "Shift+Tab" or "CmdOrCtrl+Z".
func ParseKey(s string) (ui.Key, ui.Modifiers, error) {
	parts := strings.Split(s, "+")
	if s == "" || parts[len(parts)-1] == "" {
		return 0, 0, errors.New("no key in " + strconv.Quote(s))
	}
	mods, err := modifiers(parts[:len(parts)-1])
	if err != nil {
		return 0, 0, err
	}
	name := parts[len(parts)-1]
	if k, ok := namedKeys[strings.ToLower(name)]; ok {
		return k, mods, nil
	}
	if r := []rune(strings.ToUpper(name)); len(r) == 1 {
		switch {
		case r[0] >= 'A' && r[0] <= 'Z':
			return ui.KeyA + ui.Key(r[0]-'A'), mods, nil
		case r[0] >= '0' && r[0] <= '9':
			return ui.Key0 + ui.Key(r[0]-'0'), mods, nil
		}
	}
	upper := strings.ToUpper(name)
	if len(upper) >= 2 && upper[0] == 'F' {
		n := 0
		for _, c := range upper[1:] {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1), mods, nil
		}
	}
	return 0, 0, errors.New("no key " + strconv.Quote(name))
}
