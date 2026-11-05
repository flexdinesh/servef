//go:build linux || darwin

package main

import (
	"slices"
	"strings"
)

var groups = []string{"repo", "web-static", "tooling-static", "site-static", "go", "frontend", "tooling", "docs", "browser", "desktop"}
var extendedGroups = []string{"browser-extended", "desktop-extended", "webkit"}

func knownSuite(suite string) bool {
	return suite == "" || suite == "correctness" || suite == "extended" || suite == "full"
}

func knownGroup(group string) bool {
	return slices.Contains(groups, group) || slices.Contains(extendedGroups, group)
}

func isHostGroup(group string) bool {
	return group == "repo" || group == "web-static" || group == "tooling-static" || group == "site-static"
}

func taskBelongsToGroup(task, group string) bool {
	owners := map[string]string{
		"fmt:check": "repo", "tidy:check": "repo", "mise:check": "repo", "contracts:check": "repo",
		"typecheck:web": "web-static", "lint": "web-static", "typecheck:root": "tooling-static", "typecheck:site": "site-static",
		"vet": "go", "test:go": "go", "test:race": "go", "test:web": "frontend", "test:release": "tooling", "build:site": "docs", "test:browser": "browser",
		"test:browser:extended": "browser-extended", "test:browser:webkit": "webkit",
		"test:desktop": "desktop", "test:desktop:e2e": "desktop", "test:desktop:extended": "desktop-extended",
	}
	prefix := "_container:"
	if isHostGroup(group) {
		prefix = "_host:"
	}
	name, ok := strings.CutPrefix(task, prefix)
	return ok && group != "" && owners[name] == group
}
