//go:build linux || darwin

package main

import (
	"slices"
	"strings"
)

var groups = []string{"repo", "web-static", "tooling-static", "site-static", "electron-static", "go", "frontend", "tooling", "docs", "browser", "electron"}

// optionalGroups run only when named with --group; check:pr and check:status
// cover groups alone (decision 0082).
var optionalGroups = []string{"native"}

func knownGroup(group string) bool {
	return slices.Contains(groups, group) || slices.Contains(optionalGroups, group)
}

func isHostGroup(group string) bool {
	return group == "repo" || group == "web-static" || group == "tooling-static" || group == "site-static" || group == "electron-static"
}

func taskBelongsToGroup(task, group string) bool {
	owners := map[string]string{
		"fmt:check": "repo", "tidy:check": "repo", "mise:check": "repo", "contracts:check": "repo",
		"typecheck:web": "web-static", "lint": "web-static", "typecheck:root": "tooling-static", "typecheck:site": "site-static",
		"typecheck:electron": "electron-static", "lint:electron": "electron-static",
		"vet": "go", "test:go": "go", "test:race": "go", "test:web": "frontend", "test:release": "tooling", "build:site": "docs", "test:browser": "browser",
		"test:browser:webkit": "browser", "test:electron": "electron", "test:native": "native", "test:native:e2e": "native",
	}
	prefix := "_container:"
	if isHostGroup(group) {
		prefix = "_host:"
	}
	name, ok := strings.CutPrefix(task, prefix)
	return ok && group != "" && owners[name] == group
}
