package navigation

import "strings"

// Format joins words with a space.
func Format(words ...string) string {
	return strings.Join(words, " ")
}
