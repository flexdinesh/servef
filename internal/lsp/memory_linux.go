package lsp

import (
	"os"
	"strconv"
	"strings"
)

// treeMemory sums resident memory of a process and its descendants, since
// servers such as typescript-language-server run helper processes.
func treeMemory(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	page := uint64(os.Getpagesize())
	var total uint64
	found := false
	for pending := []int{pid}; len(pending) > 0; {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if statm, err := os.ReadFile("/proc/" + strconv.Itoa(current) + "/statm"); err == nil {
			if fields := strings.Fields(string(statm)); len(fields) > 1 {
				if pages, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					total += pages * page
					found = true
				}
			}
		}
		id := strconv.Itoa(current)
		children, err := os.ReadFile("/proc/" + id + "/task/" + id + "/children")
		if err != nil {
			continue
		}
		for _, child := range strings.Fields(string(children)) {
			if childPID, err := strconv.Atoi(child); err == nil {
				pending = append(pending, childPID)
			}
		}
	}
	return total, found
}
