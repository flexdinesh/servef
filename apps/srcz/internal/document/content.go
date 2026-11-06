package document

import (
	"bytes"
	"io"
)

const (
	// SniffLength is the prefix inspected for binary content.
	SniffLength = 8000
	// PreviewLimit is the largest file size that is previewed, formatted, or
	// highlighted. Larger files show escaped plain source.
	PreviewLimit = 4 << 20
	// SearchLimit is the largest file size included in content search.
	SearchLimit = 8 << 20
	// DisplayLimit is the largest text file read for display. Larger non-image
	// files show a "Too large to display" note.
	DisplayLimit = 32 << 20
)

// IsBinary reports whether content has a NUL byte within its first SniffLength bytes.
func IsBinary(content []byte) bool {
	if len(content) > SniffLength {
		content = content[:SniffLength]
	}
	return bytes.IndexByte(content, 0) >= 0
}

// Availability is what the size and binary limits leave of a file's views.
type Availability struct {
	// TooLarge files are not read; only a note is shown.
	TooLarge          bool
	PreviewDisabled   bool
	HighlightDisabled bool
	SearchExcluded    bool
}

// DisplayReadLimit is the largest file of a type read for display, or -1 for
// none: image previews use raw bytes, so they display at any size.
func DisplayReadLimit(fileType Type) int64 {
	if fileType.PreviewKind == PreviewImage {
		return -1
	}
	return DisplayLimit
}

// Available applies the size and binary limits to a file of fileType. Image
// previews need no server work, so they stay enabled at any size; large SVG
// source is still shown, unhighlighted.
func Available(fileType Type, size int64, binary bool) Availability {
	if limit := DisplayReadLimit(fileType); limit >= 0 && size > limit {
		return Availability{TooLarge: true, PreviewDisabled: true, HighlightDisabled: true, SearchExcluded: true}
	}
	large := size > PreviewLimit
	return Availability{
		PreviewDisabled:   large && fileType.PreviewKind != PreviewImage,
		HighlightDisabled: large || binary,
		SearchExcluded:    binary || size > SearchLimit,
	}
}

// ReadText sniffs r's prefix, then reads its text up to limit bytes in total
// (when limit >= 0). A binary file stops at the prefix, which it returns.
func ReadText(r io.Reader, limit int64) (text []byte, binary bool, err error) {
	prefix, err := io.ReadAll(io.LimitReader(r, SniffLength))
	if err != nil {
		return nil, false, err
	}
	if IsBinary(prefix) {
		return prefix, true, nil
	}
	if limit >= 0 {
		r = io.LimitReader(r, max(limit-int64(len(prefix)), 0))
	}
	rest, err := io.ReadAll(r)
	if err != nil {
		return nil, false, err
	}
	return append(prefix, rest...), false, nil
}
