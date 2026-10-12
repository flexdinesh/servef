package document

import "bytes"

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
