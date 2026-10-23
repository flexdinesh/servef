package git

import (
	"bytes"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	// lineDiffTimeout bounds one line diff; past it the remaining span counts
	// as removed and added, a correct but larger edit.
	lineDiffTimeout = time.Second
	// surrogates are the runes a line id skips: UTF-16 surrogate halves are
	// not valid runes.
	surrogateStart = 0xD800
	surrogateCount = 0x800
	// lineIDs is how many distinct lines a diff can tell apart.
	lineIDs = utf8.MaxRune + 1 - surrogateCount
)

// LineChanges counts the lines a line diff from old to new adds and
// removes, as git diff --numstat does. A line ends after "\n"; a last line
// without one differs from the same text with one.
func LineChanges(old, new []byte) (added, deleted int) {
	oldLines, newLines := splitLines(old), splitLines(new)
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && bytes.Equal(oldLines[prefix], newLines[prefix]) {
		prefix++
	}
	oldLines, newLines = oldLines[prefix:], newLines[prefix:]
	suffix := 0
	for suffix < len(oldLines) && suffix < len(newLines) && bytes.Equal(oldLines[len(oldLines)-1-suffix], newLines[len(newLines)-1-suffix]) {
		suffix++
	}
	oldLines, newLines = oldLines[:len(oldLines)-suffix], newLines[:len(newLines)-suffix]
	if len(oldLines) == 0 || len(newLines) == 0 {
		return len(newLines), len(oldLines)
	}

	// Each distinct line becomes one rune, so a rune diff is a line diff.
	ids := map[string]rune{}
	encode := func(lines [][]byte) ([]rune, bool) {
		runes := make([]rune, len(lines))
		for i, line := range lines {
			id, ok := ids[string(line)]
			if !ok {
				if len(ids) == lineIDs {
					return nil, false
				}
				id = rune(len(ids))
				if id >= surrogateStart {
					id += surrogateCount
				}
				ids[string(line)] = id
			}
			runes[i] = id
		}
		return runes, true
	}
	oldRunes, oldOK := encode(oldLines)
	newRunes, newOK := encode(newLines)
	if !oldOK || !newOK {
		return len(newLines), len(oldLines)
	}
	differ := diffmatchpatch.New()
	differ.DiffTimeout = lineDiffTimeout
	for _, diff := range differ.DiffMainRunes(oldRunes, newRunes, false) {
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			added += utf8.RuneCountInString(diff.Text)
		case diffmatchpatch.DiffDelete:
			deleted += utf8.RuneCountInString(diff.Text)
		case diffmatchpatch.DiffEqual:
		}
	}
	return added, deleted
}

// splitLines splits content after each "\n", keeping it; a last line
// without one is kept as it is.
func splitLines(content []byte) [][]byte {
	var lines [][]byte
	for len(content) > 0 {
		end := bytes.IndexByte(content, '\n') + 1
		if end == 0 {
			end = len(content)
		}
		lines = append(lines, content[:end])
		content = content[end:]
	}
	return lines
}
