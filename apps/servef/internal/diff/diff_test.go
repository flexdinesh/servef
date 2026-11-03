package diff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// numbered is lines "line <from>" to "line <to-1>", one per line.
func numbered(from, to int) []string {
	var lines []string
	for i := from; i < to; i++ {
		lines = append(lines, fmt.Sprintf("line %d\n", i))
	}
	return lines
}

func context(oldStart, newStart, lines int) Block {
	return Block{OldStart: oldStart, OldCount: lines, NewStart: newStart, NewCount: lines}
}

func change(oldStart, oldCount, newStart, newCount int) Block {
	return Block{Change: true, OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}
}

// The expected hunks are what jsdiff's structuredPatch and Pierre's patch
// parser produce for the same texts (Pierre's header starts name the line
// before an empty side; these name the line after).
func TestLinesLaysOutHunksAsJSDiff(t *testing.T) {
	sixty := numbered(0, 60)
	edited := append([]string(nil), sixty...)
	edited[5], edited[30], edited[38] = "changed 5\n", "changed 30\n", "changed 38\n"
	edited = append(edited[:50], edited[51:]...)
	joined := append([]string(nil), sixty...)
	joined[10], joined[19] = "x\n", "y\n"
	split := append([]string(nil), sixty...)
	split[10], split[20] = "x\n", "y\n"

	tests := []struct {
		name     string
		old, new string
		options  Options
		want     Result
	}{
		{name: "identical", old: "a\nb\n", new: "a\nb\n"},
		{name: "both empty"},
		{name: "one line", old: "a\nb\n", new: "a\nc\n", want: Result{Additions: 1, Deletions: 1, Hunks: []Hunk{
			{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2, Additions: 1, Deletions: 1, Blocks: []Block{context(1, 1, 1), change(2, 1, 2, 1)}},
		}}},
		{name: "added file", old: "", new: "a\nb\n", want: Result{Additions: 2, Hunks: []Hunk{
			{OldStart: 1, NewStart: 1, NewCount: 2, Additions: 2, Blocks: []Block{change(1, 0, 1, 2)}},
		}}},
		{name: "deleted file", old: "a\nb\n", new: "", want: Result{Deletions: 2, Hunks: []Hunk{
			{OldStart: 1, OldCount: 2, NewStart: 1, Deletions: 2, Blocks: []Block{change(1, 2, 1, 0)}},
		}}},
		{name: "missing final newline", old: "a\nb", new: "a\nb\n", want: Result{Additions: 1, Deletions: 1, Hunks: []Hunk{
			{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2, Additions: 1, Deletions: 1, Blocks: []Block{context(1, 1, 1), change(2, 1, 2, 1)}},
		}}},
		{name: "whitespace ignored", old: "a\n  b\nc\n", new: "a\nb  \nc\n", options: Options{IgnoreWhitespace: true}},
		{name: "whitespace compared", old: "a\n  b\nc\n", new: "a\nb  \nc\n", want: Result{Additions: 1, Deletions: 1, Hunks: []Hunk{
			{OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3, Additions: 1, Deletions: 1, Blocks: []Block{context(1, 1, 1), change(2, 1, 2, 1), context(3, 3, 1)}},
		}}},
		{name: "insertions paired by similarity", old: "x\nfoo(1)\ny\n", new: "x\nnew line\nanother\nfoo(2)\ny\n", want: Result{Additions: 3, Deletions: 1, Hunks: []Hunk{
			{OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 5, Additions: 3, Deletions: 1, Blocks: []Block{context(1, 1, 1), change(2, 0, 2, 2), change(2, 1, 4, 1), context(3, 5, 1)}},
		}}},
		{name: "deletions paired by similarity", old: "x\nalpha\nbeta\nfunction go() {\ny\n", new: "x\nfunction go(){\ny\n", want: Result{Additions: 1, Deletions: 3, Hunks: []Hunk{
			{OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 3, Additions: 1, Deletions: 3, Blocks: []Block{context(1, 1, 1), change(2, 2, 2, 0), change(4, 1, 2, 1), context(5, 3, 1)}},
		}}},
		{name: "gaps between hunks", old: strings.Join(sixty, ""), new: strings.Join(edited, ""), want: Result{Additions: 3, Deletions: 4,
			Hunks: []Hunk{
				{OldStart: 2, OldCount: 9, NewStart: 2, NewCount: 9, Additions: 1, Deletions: 1, Blocks: []Block{context(2, 2, 4), change(6, 1, 6, 1), context(7, 7, 4)}},
				{OldStart: 27, OldCount: 17, NewStart: 27, NewCount: 17, Additions: 2, Deletions: 2, Blocks: []Block{context(27, 27, 4), change(31, 1, 31, 1), context(32, 32, 7), change(39, 1, 39, 1), context(40, 40, 4)}},
				{OldStart: 47, OldCount: 9, NewStart: 47, NewCount: 8, Deletions: 1, Blocks: []Block{context(47, 47, 4), change(51, 1, 51, 0), context(52, 51, 4)}},
			},
			Gaps: []Gap{
				{Hunk: 0, OldStart: 1, NewStart: 1, Lines: 1},
				{Hunk: 1, OldStart: 11, NewStart: 11, Lines: 16},
				{Hunk: 2, OldStart: 44, NewStart: 44, Lines: 3},
				{Hunk: 3, OldStart: 56, NewStart: 55, Lines: 5},
			},
		}},
		{name: "eight unchanged lines join hunks", old: strings.Join(sixty, ""), new: strings.Join(joined, ""), want: Result{Additions: 2, Deletions: 2,
			Hunks: []Hunk{
				{OldStart: 7, OldCount: 18, NewStart: 7, NewCount: 18, Additions: 2, Deletions: 2, Blocks: []Block{context(7, 7, 4), change(11, 1, 11, 1), context(12, 12, 8), change(20, 1, 20, 1), context(21, 21, 4)}},
			},
			Gaps: []Gap{{Hunk: 0, OldStart: 1, NewStart: 1, Lines: 6}, {Hunk: 1, OldStart: 25, NewStart: 25, Lines: 36}},
		}},
		{name: "nine unchanged lines split hunks", old: strings.Join(sixty, ""), new: strings.Join(split, ""), want: Result{Additions: 2, Deletions: 2,
			Hunks: []Hunk{
				{OldStart: 7, OldCount: 9, NewStart: 7, NewCount: 9, Additions: 1, Deletions: 1, Blocks: []Block{context(7, 7, 4), change(11, 1, 11, 1), context(12, 12, 4)}},
				{OldStart: 17, OldCount: 9, NewStart: 17, NewCount: 9, Additions: 1, Deletions: 1, Blocks: []Block{context(17, 17, 4), change(21, 1, 21, 1), context(22, 22, 4)}},
			},
			Gaps: []Gap{{Hunk: 0, OldStart: 1, NewStart: 1, Lines: 6}, {Hunk: 1, OldStart: 16, NewStart: 16, Lines: 1}, {Hunk: 2, OldStart: 26, NewStart: 26, Lines: 35}},
		}},
		// jsdiff keeps the first of two equal lines; trimming a common suffix
		// first would keep the last.
		{name: "repeated lines keep the earliest match", old: "a\na\n", new: "bz\na\n", want: Result{Additions: 1, Deletions: 1, Hunks: []Hunk{
			{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2, Additions: 1, Deletions: 1, Blocks: []Block{change(1, 0, 1, 1), context(1, 2, 1), change(2, 1, 3, 0)}},
		}}},
	}
	for _, test := range tests {
		got := Lines([]byte(test.old), []byte(test.new), test.options)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", test.name, got, test.want)
		}
	}
}

// Many edits far apart need a long search; each change keeps its own hunk.
func TestLinesSeparatesManyDistantChanges(t *testing.T) {
	var old, changed strings.Builder
	for line := 1; line <= 2000; line++ {
		fmt.Fprintf(&old, "line %d\n", line)
		if line%20 == 10 {
			fmt.Fprintf(&changed, "changed %d\n", line)
		} else {
			fmt.Fprintf(&changed, "line %d\n", line)
		}
	}
	result := Lines([]byte(old.String()), []byte(changed.String()), Options{})
	if len(result.Hunks) != 100 || result.Additions != 100 || result.Deletions != 100 {
		t.Fatalf("Lines = %d hunks, +%d -%d; want 100 hunks, +100 -100", len(result.Hunks), result.Additions, result.Deletions)
	}
	for i, hunk := range result.Hunks {
		want := Hunk{OldStart: 20*i + 6, OldCount: 9, NewStart: 20*i + 6, NewCount: 9, Additions: 1, Deletions: 1, Blocks: []Block{
			{OldStart: 20*i + 6, OldCount: 4, NewStart: 20*i + 6, NewCount: 4},
			{Change: true, OldStart: 20*i + 10, OldCount: 1, NewStart: 20*i + 10, NewCount: 1},
			{OldStart: 20*i + 11, OldCount: 4, NewStart: 20*i + 11, NewCount: 4},
		}}
		if !reflect.DeepEqual(hunk, want) {
			t.Fatalf("hunk %d = %+v, want %+v", i, hunk, want)
		}
	}
}

func TestLinesBoundsChangeRuns(t *testing.T) {
	defer func(previous int) { maxChanges = previous }(maxChanges)
	maxChanges = 2
	old := strings.Join(numbered(0, 40), "")
	edited := numbered(0, 40)
	edited[5], edited[20], edited[35] = "x\n", "y\n", "z\n"
	got := Lines([]byte(old), []byte(strings.Join(edited, "")), Options{})
	// One change from the first difference to the last.
	want := Result{Additions: 31, Deletions: 31,
		Hunks: []Hunk{{OldStart: 2, OldCount: 39, NewStart: 2, NewCount: 39, Additions: 31, Deletions: 31, Blocks: []Block{context(2, 2, 4), change(6, 31, 6, 31), context(37, 37, 4)}}},
		Gaps:  []Gap{{Hunk: 0, OldStart: 1, NewStart: 1, Lines: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines past maxChanges = %+v, want %+v", got, want)
	}
}

func TestLinesFallBackPastTheSearchTimeout(t *testing.T) {
	defer func(previous time.Duration) { myersTimeout = previous }(myersTimeout)
	myersTimeout = -time.Second
	old := strings.Join(numbered(0, 40), "")
	edited := numbered(0, 40)
	edited[5] = "x\n"
	got := Lines([]byte(old), []byte(strings.Join(edited, "")), Options{})
	want := Result{Additions: 1, Deletions: 1,
		Hunks: []Hunk{{OldStart: 2, OldCount: 9, NewStart: 2, NewCount: 9, Additions: 1, Deletions: 1, Blocks: []Block{context(2, 2, 4), change(6, 1, 6, 1), context(7, 7, 4)}}},
		Gaps:  []Gap{{Hunk: 0, OldStart: 1, NewStart: 1, Lines: 1}, {Hunk: 1, OldStart: 11, NewStart: 11, Lines: 30}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines by bisection = %+v, want %+v", got, want)
	}
}

func TestCountCountsLinesAsNumstat(t *testing.T) {
	many := func(from, to int) string {
		var lines strings.Builder
		for i := from; i < to; i++ {
			fmt.Fprintf(&lines, "line %d\n", i)
		}
		return lines.String()
	}
	tests := []struct {
		name           string
		old, new       string
		added, deleted int
	}{
		{name: "identical", old: "a\nb\n", new: "a\nb\n"},
		{name: "both empty", old: "", new: ""},
		{name: "added file", old: "", new: "a\nb\nc\n", added: 3},
		{name: "deleted file", old: "a\nb\n", new: "", deleted: 2},
		{name: "one line changed", old: "a\nb\nc\n", new: "a\nB\nc\n", added: 1, deleted: 1},
		{name: "insert in middle", old: "a\nc\n", new: "a\nb\nc\n", added: 1},
		{name: "remove in middle", old: "a\nb\nc\n", new: "a\nc\n", deleted: 1},
		{name: "missing final newline", old: "a\nb", new: "a\nb\n", added: 1, deleted: 1},
		{name: "no newlines", old: "a", new: "b", added: 1, deleted: 1},
		{name: "repeated lines", old: "x\nx\nx\n", new: "x\nx\n", deleted: 1},
		{name: "moved block", old: "a\nb\nc\nd\n", new: "c\nd\na\nb\n", added: 2, deleted: 2},
		{name: "many lines", old: many(0, 300), new: many(0, 100) + "inserted\n" + many(150, 300), added: 1, deleted: 50},
	}
	for _, test := range tests {
		added, deleted := Count([]byte(test.old), []byte(test.new))
		if added != test.added || deleted != test.deleted {
			t.Errorf("%s: Count = +%d -%d, want +%d -%d", test.name, added, deleted, test.added, test.deleted)
		}
	}

	// The counts agree with git's own numstat when git is installed.
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, test := range tests {
		oldName, newName := filepath.Join(dir, "old"), filepath.Join(dir, "new")
		writeFile(t, oldName, test.old)
		writeFile(t, newName, test.new)
		cmd := exec.Command(gitBinary, "diff", "--no-index", "--numstat", "old", "new")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir, "XDG_CONFIG_HOME="+dir)
		output, _ := cmd.Output()
		var added, deleted int
		if fields := strings.Fields(string(output)); len(fields) >= 2 {
			if _, err := fmt.Sscanf(fields[0]+" "+fields[1], "%d %d", &added, &deleted); err != nil {
				t.Fatalf("%s: git numstat %q: %v", test.name, output, err)
			}
		}
		if added != test.added || deleted != test.deleted {
			t.Errorf("%s: git numstat = +%d -%d, want +%d -%d", test.name, added, deleted, test.added, test.deleted)
		}
	}
}
