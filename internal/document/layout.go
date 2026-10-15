package document

import (
	"bytes"
	"strings"
)

// textLayout builds reindented formatter output. Callers write tokens
// verbatim and only decide line breaks, indentation, and single spaces between
// tokens, so formatting never rewrites values.
type textLayout struct {
	out        bytes.Buffer
	indent     int  // indent applied to the next line start
	lineIndent int  // indent of the current line
	lineStart  bool // nothing written on the current line yet
	space      bool // pending single space
	breaks     int  // pending line breaks; 2 leaves one blank line
	last       string
}

func newTextLayout() *textLayout { return &textLayout{lineStart: true} }

// Space requests one space before the next token on the same line.
func (w *textLayout) Space() {
	if !w.lineStart {
		w.space = true
	}
}

// Break requests n line breaks (at most 2) before the next token.
func (w *textLayout) Break(n int) {
	if w.out.Len() == 0 {
		return
	}
	w.breaks = max(w.breaks, min(n, 2))
}

// Write emits text verbatim, applying pending breaks, indentation, and space.
func (w *textLayout) Write(text string) {
	if text == "" {
		return
	}
	if w.breaks > 0 {
		w.out.WriteString(strings.Repeat("\n", w.breaks))
		w.lineStart, w.space, w.breaks = true, false, 0
	}
	if w.lineStart {
		w.lineIndent = max(w.indent, 0)
		w.out.WriteString(strings.Repeat("  ", w.lineIndent))
	} else if w.space {
		w.out.WriteByte(' ')
	}
	w.out.WriteString(text)
	w.lineStart, w.space, w.last = false, false, text
}

// WriteTrailing keeps text (a comment) on the current line even when a break
// was requested, then restores the pending break.
func (w *textLayout) WriteTrailing(text string) {
	pending := w.breaks
	w.breaks = 0
	w.Space()
	w.Write(text)
	w.Break(pending)
}

// Bytes returns the output with exactly one trailing newline when non-empty.
func (w *textLayout) Bytes() []byte {
	output := bytes.TrimRight(w.out.Bytes(), " \t\n")
	if len(output) == 0 {
		return []byte{}
	}
	return append(output, '\n')
}
