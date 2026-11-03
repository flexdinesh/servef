package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/servef/internal/diff"
	"github.com/flexdinesh/servef/pkg/contract"
)

func TestSourceControlFailuresAreTypedWithoutTransport(t *testing.T) {
	application, root := newTestApp(t)
	ctx := context.Background()
	sc := application.SourceControl

	changes, err := sc.Changes(ctx, root)
	if err != nil || changes.Repository || changes.Files == nil {
		t.Errorf("Changes of a folder = %+v, %v; want no repository and empty files", changes, err)
	}
	cases := map[string]struct {
		call func() error
		kind contract.ErrorKind
	}{
		"Changes without a project": {func() error { _, err := sc.Changes(ctx, ""); return err }, contract.KindInvalid},
		"Diff without a path": {func() error {
			_, err := sc.Diff(ctx, contract.DiffRequest{Project: root})
			return err
		}, contract.KindInvalid},
		"Diff with a base outside a branch diff": {func() error {
			_, err := sc.Diff(ctx, contract.DiffRequest{Project: root, Path: "data.json", Base: "main"})
			return err
		}, contract.KindInvalid},
		"Diff of a commit by name": {func() error {
			_, err := sc.Diff(ctx, contract.DiffRequest{Project: root, Kind: contract.DiffCommit, Commit: "HEAD", Path: "data.json"})
			return err
		}, contract.KindInvalid},
		"Commits with an invalid limit": {func() error {
			_, err := sc.Commits(ctx, contract.CommitsRequest{Project: root, Limit: -1})
			return err
		}, contract.KindInvalid},
		"CommitFiles by name": {func() error { _, err := sc.CommitFiles(ctx, root, "HEAD"); return err }, contract.KindInvalid},
		"BranchFiles with a head and uncommitted": {func() error {
			_, err := sc.BranchFiles(ctx, contract.BranchFilesRequest{Project: root, Head: "feature", Uncommitted: true})
			return err
		}, contract.KindInvalid},
		"CompareStats of an unknown compare": {func() error {
			_, err := sc.CompareStats(ctx, contract.CompareStatsRequest{Project: root, Compare: "nope"})
			return err
		}, contract.KindInvalid},
		"CompareStats with a stray parameter": {func() error {
			_, err := sc.CompareStats(ctx, contract.CompareStatsRequest{Project: root, Compare: contract.CompareWorking, From: "a"})
			return err
		}, contract.KindInvalid},
	}
	for name, tc := range cases {
		if kind := kindOf(t, tc.call()); kind != tc.kind {
			t.Errorf("%s error kind = %s, want %s", name, kind, tc.kind)
		}
	}
}

// The frontend's diff fixtures carry the line diff the server computes for
// their contents, so web tests exercise real server hunks.
func TestDiffFixturesCarryTheServerLineDiff(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "testdata", "api", "default", "diff-*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("diff fixtures = %v, %v", names, err)
	}
	for _, name := range names {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var fixture contract.DiffResponse
		if err := json.Unmarshal(content, &fixture); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var want *contract.LineDiff
		original, modified := sideContent(fixture.Original), sideContent(fixture.Modified)
		if (fixture.Original == nil || original != nil) && (fixture.Modified == nil || modified != nil) {
			want = lineDiff(diff.Lines(original, modified, diff.Options{}), false)
		}
		if !reflect.DeepEqual(fixture.LineDiff, want) {
			t.Errorf("%s lineDiff = %+v, want %+v", filepath.Base(name), fixture.LineDiff, want)
		}
	}
}

// sideContent is a side's text, nil when the side is absent or carries none.
func sideContent(side *contract.DiffSide) []byte {
	if side == nil || side.Content == nil {
		return nil
	}
	return []byte(*side.Content)
}
