//go:build linux || darwin

package main

import "strings"

var groups = []string{"repo", "web-static", "tooling-static", "site-static", "go", "frontend", "tooling", "docs", "browser"}

func isHostGroup(group string) bool {
	return group == "repo" || group == "web-static" || group == "tooling-static" || group == "site-static"
}

func taskBelongsToGroup(task, group string) bool {
	owners := map[string]string{
		"fmt:check": "repo", "tidy:check": "repo", "mise:check": "repo",
		"typecheck:web": "web-static", "lint": "web-static", "typecheck:root": "tooling-static", "typecheck:site": "site-static",
		"vet": "go", "test:go": "go", "test:race": "go", "test:web": "frontend", "test:release": "tooling", "build:site": "docs", "test:browser": "browser",
	}
	prefix := "_container:"
	if isHostGroup(group) {
		prefix = "_host:"
	}
	name, ok := strings.CutPrefix(task, prefix)
	return ok && group != "" && owners[name] == group
}
