// Command contracts generates packages/contracts, the TypeScript types and
// runtime guards for servef's transport contract, from apps/servef/pkg/contract
// (decision 0080). Run it from the repository root:
//
//	go run ./tools/contracts          # write packages/contracts/src
//	go run ./tools/contracts -check   # fail when the committed output drifts
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	sourceDir = "apps/servef/pkg/contract"
	outputDir = "packages/contracts/src"
)

// generated lists the contract files emitted as TypeScript, in index order.
var generated = []string{"errors.go", "projects.go", "documents.go"}

// goOnly lists contract files with no generated TypeScript: package docs,
// transport mappings and request parameters, and shapes whose TypeScript
// guards are still hand-written until they migrate.
var goOnly = []string{"doc.go", "kinds.go", "requests.go", "events.go", "daemon.go"}

func main() {
	check := flag.Bool("check", false, "fail when "+outputDir+" differs from the generated output")
	flag.Parse()
	if err := run(".", *check); err != nil {
		fmt.Fprintln(os.Stderr, "contracts:", err)
		os.Exit(1)
	}
}

func run(root string, check bool) error {
	m, err := load(filepath.Join(root, sourceDir), generated, goOnly)
	if err != nil {
		return err
	}
	output := emit(m)
	dir := filepath.Join(root, outputDir)
	if check {
		return compare(dir, output)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stale, err := extraFiles(dir, output)
	if err != nil {
		return err
	}
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(output) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(output[name]), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// compare reports every file that differs from, is missing from, or is not
// in the generated output.
func compare(dir string, output map[string]string) error {
	var drift []string
	for _, name := range sortedKeys(output) {
		current, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(current, []byte(output[name])) {
			drift = append(drift, filepath.Join(outputDir, name))
		}
	}
	extra, err := extraFiles(dir, output)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, name := range extra {
		drift = append(drift, filepath.Join(outputDir, name)+" (not generated)")
	}
	if len(drift) > 0 {
		return fmt.Errorf("generated contracts are stale; run mise run contracts:generate:\n  %s", strings.Join(drift, "\n  "))
	}
	return nil
}

func extraFiles(dir string, output map[string]string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var extra []string
	for _, entry := range entries {
		if _, ok := output[entry.Name()]; !ok {
			extra = append(extra, entry.Name())
		}
	}
	slices.Sort(extra)
	return extra, nil
}
