package diffs

import (
	"time"
	"unicode"
	"unicode/utf8"
)

// Inline is how changed line pairs emphasize what changed within them.
type Inline uint8

const (
	// WordAlt emphasizes changed words, joining runs one character
	// apart, as Pierre's default "word-alt".
	WordAlt Inline = iota
	// Word emphasizes each changed word.
	Word
	// Char emphasizes changed characters.
	Char
	// NoInline emphasizes nothing.
	NoInline
)

// maxEmphasisLength is Pierre's maxLineDiffLength: longer lines get no
// emphasis.
const maxEmphasisLength = 1000

// Range is the bytes From to To (exclusive) of a line.
type Range struct{ From, To int }

// Emphasis compares a changed pair, the old line and the new one without
// their line endings, as Pierre does (jsdiff's diffWordsWithSpace or
// diffChars), and returns each side's changed runs. It only presents a
// pair the diff made; it never changes which lines changed. A pair past
// the search's bounds (maxEmphasisEdits, emphasisTimeout) gets none.
func Emphasis(old, new string, mode Inline) (oldRanges, newRanges []Range) {
	if mode == NoInline || utf16Len(old) > maxEmphasisLength || utf16Len(new) > maxEmphasisLength {
		return nil, nil
	}
	tokenize := wordTokens
	if mode == Char {
		tokenize = charTokens
	}
	changes, ok := diffTokens(old, new, tokenize(old), tokenize(new), maxEmphasisEdits, time.Now().Add(emphasisTimeout))
	if !ok {
		return nil, nil
	}
	if mode == Word || mode == Char {
		// Pierre joins nothing but neighbors of one kind.
		return runs(changes, false)
	}
	return runs(changes, true)
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// change is a run of jsdiff's result: kept, added, or removed text.
type change struct {
	value          string
	added, removed bool
}

// runs places changes on each side as Pierre's pushOrJoinSpan does: with
// join, a one-character unchanged run between changes, and changes
// following each other, join one emphasized run.
func runs(changes []change, join bool) (oldRanges, newRanges []Range) {
	type piece struct {
		emphasized bool
		length     int
	}
	var oldPieces, newPieces []piece
	push := func(pieces []piece, c change, neutral, last bool) []piece {
		length := len(c.value)
		if len(pieces) == 0 || last || !join {
			return append(pieces, piece{!neutral, length})
		}
		prev := &pieces[len(pieces)-1]
		prevNeutral := !prev.emphasized
		if neutral == prevNeutral || neutral && utf16Len(c.value) == 1 && !prevNeutral {
			prev.length += length
			return pieces
		}
		return append(pieces, piece{!neutral, length})
	}
	for i, c := range changes {
		last := i == len(changes)-1
		switch {
		case !c.added && !c.removed:
			oldPieces = push(oldPieces, c, true, last)
			newPieces = push(newPieces, c, true, last)
		case c.removed:
			oldPieces = push(oldPieces, c, false, last)
		default:
			newPieces = push(newPieces, c, false, last)
		}
	}
	ranges := func(pieces []piece) []Range {
		var out []Range
		at := 0
		for _, p := range pieces {
			if p.emphasized && p.length > 0 {
				out = append(out, Range{at, at + p.length})
			}
			at += p.length
		}
		return out
	}
	return ranges(oldPieces), ranges(newPieces)
}

// isWordChar is jsdiff's extendedWordChars.
func isWordChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		return true
	case r == 0xAD, r >= 0xC0 && r <= 0xD6, r >= 0xD8 && r <= 0xF6, r >= 0xF8 && r <= 0x2C6,
		r >= 0x2C8 && r <= 0x2D7, r >= 0x2DE && r <= 0x2FF, r >= 0x1E00 && r <= 0x1EFF:
		return true
	}
	return false
}

// isSpace is JavaScript's \s but for line breaks.
func isSpace(r rune) bool {
	if r == '\n' || r == '\r' {
		return false
	}
	switch r {
	case '\t', '\v', '\f', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A || unicode.Is(unicode.Zs, r)
}

// wordTokens is jsdiff's WordsWithSpaceDiff tokenizer: each line break, a
// run of word characters, a run of spaces, or any other one character.
func wordTokens(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		j := i + size
		switch {
		case r == '\r' && j < len(s) && s[j] == '\n':
			j++
		case r == '\n':
		case isWordChar(r):
			for j < len(s) {
				r, size := utf8.DecodeRuneInString(s[j:])
				if !isWordChar(r) {
					break
				}
				j += size
			}
		case isSpace(r):
			for j < len(s) {
				r, size := utf8.DecodeRuneInString(s[j:])
				if !isSpace(r) {
					break
				}
				j += size
			}
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// charTokens is one token per code point, as jsdiff's diffChars.
func charTokens(s string) []string {
	out := make([]string, 0, len(s))
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		out = append(out, s[i:i+size])
		i += size
	}
	return out
}

// component is a run of an edit path, linked to the run before it.
type component struct {
	count          int
	added, removed bool
	previous       *component
}

type editPath struct {
	oldPos int
	last   *component
}

// Bounds of one pair's search, as jsdiff's maxEditLength and timeout: a
// pair needing more edits, or more time, gets no emphasis. Lines this
// different show as wholly changed by their tint alone.
const (
	maxEmphasisEdits   = 400
	emphasisTimeout    = 20 * time.Millisecond
	deadlineCheckEvery = 64
)

// diffTokens is jsdiff's base diff (Myers's greedy search with jsdiff's
// choice among equal paths) over tokens, the consecutive pieces of old
// and new. It gives up, reporting false, past maxEdits edits or deadline.
func diffTokens(old, new string, oldTokens, newTokens []string, maxEdits int, deadline time.Time) ([]change, bool) {
	oldLen, newLen := len(oldTokens), len(newTokens)
	offsets := func(tokens []string) []int {
		at := make([]int, len(tokens)+1)
		for i, t := range tokens {
			at[i+1] = at[i] + len(t)
		}
		return at
	}
	oldAt, newAt := offsets(oldTokens), offsets(newTokens)
	extract := func(p *editPath, diagonal int) int {
		oldPos := p.oldPos
		newPos := oldPos - diagonal
		common := 0
		for newPos+1 < newLen && oldPos+1 < oldLen && oldTokens[oldPos+1] == newTokens[newPos+1] {
			newPos++
			oldPos++
			common++
		}
		if common > 0 {
			p.last = &component{count: common, previous: p.last}
		}
		p.oldPos = oldPos
		return newPos
	}
	add := func(p *editPath, added, removed bool, oldInc int) *editPath {
		if l := p.last; l != nil && l.added == added && l.removed == removed {
			return &editPath{oldPos: p.oldPos + oldInc, last: &component{count: l.count + 1, added: added, removed: removed, previous: l.previous}}
		}
		return &editPath{oldPos: p.oldPos + oldInc, last: &component{count: 1, added: added, removed: removed, previous: p.last}}
	}
	build := func(last *component) []change {
		n := 0
		for c := last; c != nil; c = c.previous {
			n++
		}
		comps := make([]*component, n)
		for c := last; c != nil; c = c.previous {
			n--
			comps[n] = c
		}
		out := make([]change, 0, len(comps))
		oldPos, newPos := 0, 0
		for _, c := range comps {
			switch {
			case c.removed:
				out = append(out, change{value: old[oldAt[oldPos]:oldAt[oldPos+c.count]], removed: true})
				oldPos += c.count
			case c.added:
				out = append(out, change{value: new[newAt[newPos]:newAt[newPos+c.count]], added: true})
				newPos += c.count
			default:
				out = append(out, change{value: new[newAt[newPos]:newAt[newPos+c.count]]})
				newPos += c.count
				oldPos += c.count
			}
		}
		return out
	}
	maxEdit := min(oldLen+newLen, maxEdits)
	// best holds the furthest path on each diagonal, offset by maxEdit+1.
	offset := maxEdit + 1
	best := make([]*editPath, 2*offset+1)
	best[offset] = &editPath{oldPos: -1}
	newPos := extract(best[offset], 0)
	if best[offset].oldPos+1 >= oldLen && newPos+1 >= newLen {
		return build(best[offset].last), true
	}
	at := func(diagonal int) *editPath {
		if i := diagonal + offset; i >= 0 && i < len(best) {
			return best[i]
		}
		return nil
	}
	set := func(diagonal int, p *editPath) {
		if i := diagonal + offset; i >= 0 && i < len(best) {
			best[i] = p
		}
	}
	minDiagonal, maxDiagonal := -maxEdit-1, maxEdit+1
	steps := 0
	for edit := 1; edit <= maxEdit; edit++ {
		for diagonal := max(minDiagonal, -edit); diagonal <= min(maxDiagonal, edit); diagonal += 2 {
			if steps++; steps%deadlineCheckEvery == 0 && !deadline.IsZero() && time.Now().After(deadline) {
				return nil, false
			}
			removePath, addPath := at(diagonal-1), at(diagonal+1)
			if removePath != nil {
				set(diagonal-1, nil)
			}
			canAdd := false
			if addPath != nil {
				addNewPos := addPath.oldPos - diagonal
				canAdd = 0 <= addNewPos && addNewPos < newLen
			}
			canRemove := removePath != nil && removePath.oldPos+1 < oldLen
			if !canAdd && !canRemove {
				set(diagonal, nil)
				continue
			}
			var base *editPath
			if !canRemove || canAdd && removePath.oldPos < addPath.oldPos {
				base = add(addPath, true, false, 0)
			} else {
				base = add(removePath, false, true, 1)
			}
			newPos = extract(base, diagonal)
			if base.oldPos+1 >= oldLen && newPos+1 >= newLen {
				return build(base.last), true
			}
			set(diagonal, base)
			if base.oldPos+1 >= oldLen {
				maxDiagonal = min(maxDiagonal, diagonal-1)
			}
			if newPos+1 >= newLen {
				minDiagonal = max(minDiagonal, diagonal+1)
			}
		}
	}
	return nil, false
}
