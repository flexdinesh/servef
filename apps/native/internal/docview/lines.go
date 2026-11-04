package docview

import (
	"fmt"
	"regexp"
	"strconv"
)

// LineRange is an inclusive, 1-based range of source lines, as
// lib/line-selection.ts. A nil *LineRange is no selection.
type LineRange struct{ Start, End int }

var lineAnchorPattern = regexp.MustCompile(`^#?L([1-9]\d*)(?:-L?([1-9]\d*))?$`)

// maxSafeLine is JavaScript's Number.MAX_SAFE_INTEGER, which bounds the
// web's anchors.
const maxSafeLine = 1<<53 - 1

// ParseLineAnchor reads GitHub-style fragments: #L12 or #L12-L20 (#L12-20
// too); nil for anything else.
func ParseLineAnchor(hash string) *LineRange {
	match := lineAnchorPattern.FindStringSubmatch(hash)
	if match == nil {
		return nil
	}
	first, err := strconv.Atoi(match[1])
	if err != nil || first > maxSafeLine {
		return nil
	}
	second := first
	if match[2] != "" {
		if second, err = strconv.Atoi(match[2]); err != nil || second > maxSafeLine {
			return nil
		}
	}
	return &LineRange{min(first, second), max(first, second)}
}

// LineAnchor formats a range as a fragment without the leading #.
func LineAnchor(r LineRange) string {
	if r.Start == r.End {
		return fmt.Sprintf("L%d", r.Start)
	}
	return fmt.Sprintf("L%d-L%d", r.Start, r.End)
}

// ClampLines fits a range to a document, nil when it starts past the end.
func ClampLines(r *LineRange, count int) *LineRange {
	if r == nil || r.Start > count {
		return nil
	}
	return &LineRange{r.Start, min(r.End, count)}
}

// SpanLines is the inclusive range between two lines, in either order.
func SpanLines(anchor, line int) LineRange {
	return LineRange{min(anchor, line), max(anchor, line)}
}

// StartLineSelection applies a gutter press: plain presses anchor a new
// selection; Shift extends from anchor (the last plain press, 0 for none),
// else from the current start. Dragging then spans from the returned
// anchor.
func StartLineSelection(current *LineRange, anchor, line int, extend bool) (int, LineRange) {
	from := line
	if extend {
		switch {
		case anchor != 0:
			from = anchor
		case current != nil:
			from = current.Start
		}
	}
	return from, SpanLines(from, line)
}

// LineReference formats editor-style references: path:12 or path:12-20.
func LineReference(path string, r LineRange) string {
	if r.Start == r.End {
		return fmt.Sprintf("%s:%d", path, r.Start)
	}
	return fmt.Sprintf("%s:%d-%d", path, r.Start, r.End)
}

// LineRangeLabel formats the status bar's position: Ln 12 or Ln 12–20.
func LineRangeLabel(r LineRange) string {
	if r.Start == r.End {
		return fmt.Sprintf("Ln %d", r.Start)
	}
	return fmt.Sprintf("Ln %d–%d", r.Start, r.End)
}
