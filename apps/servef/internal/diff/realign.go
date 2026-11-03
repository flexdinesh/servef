package diff

import (
	"slices"
	"unicode/utf16"
)

const (
	// maxAlignmentComparisons bounds the line comparisons one change block's
	// re-pairing makes.
	maxAlignmentComparisons = 4096
	// minImprovementPerPair is the similarity per pair an offset must gain
	// over pairing from the block's start.
	minImprovementPerPair = 0.5
)

// realign re-pairs change blocks whose deletion and addition counts differ
// so deleted lines sit beside the added lines they most resemble, as
// Pierre's realignChangeContentBySimilarity does: a block becomes up to
// three, a pure run, the pairs, and a pure run. Changes never cross
// unchanged lines and every line keeps its row.
func realign(blocks []Block, oldLines, newLines [][]byte) []Block {
	var result []Block
	for _, block := range blocks {
		if !block.Change {
			result = append(result, block)
			continue
		}
		result = append(result, realignBlock(block, oldLines, newLines)...)
	}
	return result
}

func realignBlock(block Block, oldLines, newLines [][]byte) []Block {
	deletions, additions := block.OldCount, block.NewCount
	pairs := min(deletions, additions)
	surplus := max(additions-deletions, deletions-additions)
	if pairs == 0 || surplus == 0 || pairs*(surplus+1) > maxAlignmentComparisons {
		return []Block{block}
	}
	stripped := func(lines [][]byte, start, count int) [][]uint16 {
		result := make([][]uint16, count)
		for i := range result {
			result[i] = stripSpace(lines[start-1+i])
		}
		return result
	}
	removed := stripped(oldLines, block.OldStart, deletions)
	added := stripped(newLines, block.NewStart, additions)
	addedLonger := additions > deletions
	bestOffset, bestScore := 0, -1.0
	for offset := 0; offset <= surplus; offset++ {
		score := 0.0
		for pair := 0; pair < pairs; pair++ {
			if addedLonger {
				score += similarity(removed[pair], added[pair+offset])
			} else {
				score += similarity(removed[pair+offset], added[pair])
			}
		}
		if offset == 0 {
			bestScore = score + float64(pairs)*minImprovementPerPair
		} else if score > bestScore {
			bestScore, bestOffset = score, offset
		}
	}
	if bestOffset == 0 {
		return []Block{block}
	}
	var result []Block
	push := func(oldStart, oldCount, newStart, newCount int) {
		if oldCount > 0 || newCount > 0 {
			result = append(result, Block{Change: true, OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount})
		}
	}
	oldStart, newStart := block.OldStart, block.NewStart
	if addedLonger {
		push(oldStart, 0, newStart, bestOffset)
		push(oldStart, pairs, newStart+bestOffset, pairs)
		push(oldStart+pairs, 0, newStart+bestOffset+pairs, additions-pairs-bestOffset)
	} else {
		push(oldStart, bestOffset, newStart, 0)
		push(oldStart+bestOffset, pairs, newStart, pairs)
		push(oldStart+bestOffset+pairs, deletions-pairs-bestOffset, newStart+pairs, 0)
	}
	return result
}

// stripSpace removes every JavaScript whitespace character from a line and
// returns its UTF-16 code units, the units JavaScript strings compare in.
func stripSpace(line []byte) []uint16 {
	runes := []rune(string(line))
	kept := runes[:0]
	for _, r := range runes {
		if !isJSSpace(r) {
			kept = append(kept, r)
		}
	}
	return utf16.Encode(kept)
}

// similarity is the share of the longer line its common prefix and suffix
// cover: 1 for equal lines, 0 when either is empty.
func similarity(a, b []uint16) float64 {
	if slices.Equal(a, b) {
		return 1
	}
	longest, shortest := max(len(a), len(b)), min(len(a), len(b))
	if shortest == 0 {
		return 0
	}
	prefix := 0
	for prefix < shortest && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < shortest-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	return float64(prefix+suffix) / float64(longest)
}
