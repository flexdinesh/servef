// Package scenario runs YAML scenarios against a mygo app through the
// automation protocol (a headless session or a built app) and writes named
// shots: <out>/<scenario>/<shot>.png and tree.json. A scenario may also
// describe how to reach the same shots in the web app it mirrors, which
// agent/webshot captures with agent-browser.
//
//	name: counter
//	size: {width: 800, height: 600}
//	dark: false
//	steps:
//	  - click: Add                     # the element showing the text
//	  - click: {x: 20, y: 40}          # or a position; button: right, clicks: 2
//	  - fill: {label: Name, value: Ada}
//	  - type: Grace
//	  - key: CmdOrCtrl+A
//	  - scroll: {label: Rows, dy: 300} # or x, y; dx
//	  - resize: {width: 640, height: 480}
//	  - dark: true
//	  - wait: Count                    # text, or {text, timeout} or {ms}
//	  - settle: true                   # idle
//	  - expect: [Count: 1]             # texts the frame must show
//	  - shot: after-add
//	web:
//	  storage: {theme: ${theme}}       # localStorage before each shot; ${theme} is light or dark
//	  shots:
//	    after-add:
//	      path: /counter
//	      steps: [{click: Add}, {click: {label: Close, by: label}}, {wait: "Count: 1"}]
package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scenario is a parsed scenario file.
type Scenario struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Size        *Size  `yaml:"size,omitempty" json:"size,omitempty"`
	Dark        bool   `yaml:"dark,omitempty" json:"dark,omitempty"`
	Steps       []Step `yaml:"steps" json:"steps"`
	// Web reaches the shots in the web app the native app mirrors.
	Web *Web `yaml:"web,omitempty" json:"web,omitempty"`
}

// Web is a scenario's web side.
type Web struct {
	// Storage holds localStorage entries set before each shot loads; the
	// text ${theme} in a value is the shot's appearance, light or dark.
	Storage map[string]string `yaml:"storage,omitempty" json:"storage,omitempty"`
	// Shots reach native shots by name.
	Shots map[string]WebShot `yaml:"shots" json:"shots"`
}

// Size is a window size in DIPs (CSS pixels on the web).
type Size struct {
	Width  int `yaml:"width" json:"width"`
	Height int `yaml:"height" json:"height"`
}

// WebShot is how to reach a shot in the web app: a path below the base
// URL, then steps (click, fill, type, key, scroll, dark, wait, settle).
type WebShot struct {
	Path  string `yaml:"path" json:"path"`
	Steps []Step `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// Step is one action; exactly one field is set.
type Step struct {
	Click  *Click  `yaml:"click,omitempty" json:"click,omitempty"`
	Fill   *Fill   `yaml:"fill,omitempty" json:"fill,omitempty"`
	Type   *string `yaml:"type,omitempty" json:"type,omitempty"`
	Key    *string `yaml:"key,omitempty" json:"key,omitempty"`
	Scroll *Scroll `yaml:"scroll,omitempty" json:"scroll,omitempty"`
	Resize *Size   `yaml:"resize,omitempty" json:"resize,omitempty"`
	Dark   *bool   `yaml:"dark,omitempty" json:"dark,omitempty"`
	Wait   *Wait   `yaml:"wait,omitempty" json:"wait,omitempty"`
	Settle *Settle `yaml:"settle,omitempty" json:"settle,omitempty"`
	Expect Texts   `yaml:"expect,omitempty" json:"expect,omitempty"`
	Shot   *string `yaml:"shot,omitempty" json:"shot,omitempty"`
}

// Click presses the element showing Label, or clicks at X, Y. On the web,
// By finds Label as agent-browser's find does: text (the default), label,
// role, placeholder, alt, title, or testid; css takes Label as a selector.
type Click struct {
	Label  string  `yaml:"label,omitempty" json:"label,omitempty"`
	By     string  `yaml:"by,omitempty" json:"by,omitempty"`
	X      float64 `yaml:"x,omitempty" json:"x,omitempty"`
	Y      float64 `yaml:"y,omitempty" json:"y,omitempty"`
	Button string  `yaml:"button,omitempty" json:"button,omitempty"`
	Clicks int     `yaml:"clicks,omitempty" json:"clicks,omitempty"`
	// Mods are modifiers held during the click, as shift (native only).
	Mods []string `yaml:"mods,omitempty" json:"mods,omitempty"`
}

// UnmarshalYAML takes a text or a mapping.
func (c *Click) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.Label = n.Value
		return nil
	}
	type plain Click
	return decodeStrict(n, (*plain)(c))
}

// Fill replaces the text of the field labeled Label.
type Fill struct {
	Label string `yaml:"label" json:"label"`
	Value string `yaml:"value" json:"value"`
}

// Scroll scrolls by DX, DY at the element showing Label, at X, Y, or at
// the window's middle.
type Scroll struct {
	Label string  `yaml:"label,omitempty" json:"label,omitempty"`
	X     float64 `yaml:"x,omitempty" json:"x,omitempty"`
	Y     float64 `yaml:"y,omitempty" json:"y,omitempty"`
	DX    float64 `yaml:"dx,omitempty" json:"dx,omitempty"`
	DY    float64 `yaml:"dy,omitempty" json:"dy,omitempty"`
}

// UnmarshalYAML refuses unknown fields.
func (s *Scroll) UnmarshalYAML(n *yaml.Node) error {
	type plain Scroll
	return decodeStrict(n, (*plain)(s))
}

// Wait waits for a text (Timeout ms, 5000 by default) or for MS.
type Wait struct {
	Text    string `yaml:"text,omitempty" json:"text,omitempty"`
	Timeout int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	MS      int    `yaml:"ms,omitempty" json:"ms,omitempty"`
}

// UnmarshalYAML takes a text or a mapping.
func (w *Wait) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		w.Text = n.Value
		return nil
	}
	type plain Wait
	return decodeStrict(n, (*plain)(w))
}

// Settle waits until the window settles (Timeout ms, 10000 by default).
type Settle struct {
	Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// UnmarshalYAML takes true or a mapping.
func (s *Settle) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var on bool
		if err := n.Decode(&on); err != nil || !on {
			return fmt.Errorf("line %d: settle takes true or {timeout: ms}", n.Line)
		}
		return nil
	}
	type plain Settle
	return decodeStrict(n, (*plain)(s))
}

// Texts are one text or a list of them.
type Texts []string

// UnmarshalYAML takes a text or a list.
func (t *Texts) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*t = Texts{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*t = list
	return nil
}

func decodeStrict(n *yaml.Node, v any) error {
	b, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	return nil
}

// Kind names the step's action.
func (s Step) Kind() string {
	kinds := s.kinds()
	if len(kinds) == 1 {
		return kinds[0]
	}
	return ""
}

func (s Step) kinds() []string {
	var out []string
	for _, k := range []struct {
		name string
		set  bool
	}{
		{"click", s.Click != nil}, {"fill", s.Fill != nil}, {"type", s.Type != nil}, {"key", s.Key != nil},
		{"scroll", s.Scroll != nil}, {"resize", s.Resize != nil}, {"dark", s.Dark != nil}, {"wait", s.Wait != nil},
		{"settle", s.Settle != nil}, {"expect", len(s.Expect) > 0}, {"shot", s.Shot != nil},
	} {
		if k.set {
			out = append(out, k.name)
		}
	}
	return out
}

var locators = map[string]bool{"": true, "text": true, "label": true, "role": true, "placeholder": true, "alt": true, "title": true, "testid": true, "css": true}

var shotName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Parse reads a scenario; name is used when the file names none.
func Parse(data []byte, name string) (Scenario, error) {
	var sc Scenario
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&sc); err != nil {
		return Scenario{}, fmt.Errorf("scenario %s: %w", name, err)
	}
	if sc.Name == "" {
		sc.Name = name
	}
	if err := sc.Validate(); err != nil {
		return Scenario{}, err
	}
	return sc, nil
}

// Load reads a scenario file; its base name is the default name.
func Load(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	return Parse(data, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

// Validate checks names and that each step does one thing.
func (sc Scenario) Validate() error {
	if !shotName.MatchString(sc.Name) {
		return fmt.Errorf("scenario name %q: use lower case letters, digits, '.', '_' and '-'", sc.Name)
	}
	if sc.Size != nil && (sc.Size.Width <= 0 || sc.Size.Height <= 0) {
		return fmt.Errorf("scenario %s: size must be positive", sc.Name)
	}
	shots := map[string]bool{}
	var errs []error
	check := func(where string, i int, step Step, web bool) {
		kinds := step.kinds()
		if len(kinds) != 1 {
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d has %d actions %v, want one", sc.Name, where, i+1, len(kinds), kinds))
			return
		}
		switch {
		case step.Shot != nil:
			if web {
				errs = append(errs, fmt.Errorf("scenario %s: %s step %d: web steps take no shot; the web shot is named by its key", sc.Name, where, i+1))
			} else if !shotName.MatchString(*step.Shot) || shots[*step.Shot] {
				errs = append(errs, fmt.Errorf("scenario %s: shot %q must be a unique lower-case name", sc.Name, *step.Shot))
			}
			shots[*step.Shot] = true
		case step.Click != nil && step.Click.Label == "" && step.Click.X == 0 && step.Click.Y == 0:
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: click needs a label or a position", sc.Name, where, i+1))
		case step.Click != nil && !locators[step.Click.By]:
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: click by %q: use text, label, role, placeholder, alt, title, testid, or css", sc.Name, where, i+1, step.Click.By))
		case step.Click != nil && step.Click.By != "" && step.Click.By != "text" && !web:
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: native clicks find text only", sc.Name, where, i+1))
		case step.Fill != nil && step.Fill.Label == "":
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: fill needs a label", sc.Name, where, i+1))
		case step.Resize != nil && (step.Resize.Width <= 0 || step.Resize.Height <= 0):
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: resize must be positive", sc.Name, where, i+1))
		case step.Wait != nil && step.Wait.Text == "" && step.Wait.MS <= 0:
			errs = append(errs, fmt.Errorf("scenario %s: %s step %d: wait needs a text or ms", sc.Name, where, i+1))
		}
	}
	for i, step := range sc.Steps {
		check("native", i, step, false)
	}
	if len(shots) == 0 {
		errs = append(errs, fmt.Errorf("scenario %s: no shot", sc.Name))
	}
	var webShots map[string]WebShot
	if sc.Web != nil {
		webShots = sc.Web.Shots
	}
	for name, web := range webShots {
		if !shots[name] {
			errs = append(errs, fmt.Errorf("scenario %s: web shot %q names no native shot", sc.Name, name))
		}
		if !strings.HasPrefix(web.Path, "/") {
			errs = append(errs, fmt.Errorf("scenario %s: web shot %q: path must start with /", sc.Name, name))
		}
		for i, step := range web.Steps {
			check("web "+name, i, step, true)
		}
	}
	return errors.Join(errs...)
}

// Shots lists the native shot names in order.
func (sc Scenario) Shots() []string {
	var out []string
	for _, step := range sc.Steps {
		if step.Shot != nil {
			out = append(out, *step.Shot)
		}
	}
	return out
}
