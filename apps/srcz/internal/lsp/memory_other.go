//go:build !linux

package lsp

// treeMemory is unavailable without /proc.
func treeMemory(int) (uint64, bool) { return 0, false }
