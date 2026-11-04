package highlights

import "unicode"

// isIDStart reports Unicode ID_Start, as /^\p{ID_Start}$/u does:
// letters, letter numbers, and Other_ID_Start, less Pattern_Syntax and
// Pattern_White_Space.
func isIDStart(r rune) bool {
	if r < 0x80 {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
	}
	if unicode.Is(unicode.Pattern_Syntax, r) || unicode.Is(unicode.Pattern_White_Space, r) {
		return false
	}
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_ID_Start, r)
}

// isIDContinue reports Unicode ID_Continue plus ZWNJ and ZWJ, as
// /^[‌‍\p{ID_Continue}]$/u does.
func isIDContinue(r rune) bool {
	if r < 0x80 {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_'
	}
	if r == 0x200C || r == 0x200D {
		return true
	}
	if unicode.Is(unicode.Pattern_Syntax, r) || unicode.Is(unicode.Pattern_White_Space, r) {
		return false
	}
	return isIDStart(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue)
}
