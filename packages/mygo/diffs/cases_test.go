package diffs

// lineDiffCases are the line diffs the web tests build with lineDiffOf
// (apps/web/src/features/source-control/line-diff-testing.ts) for the same
// texts, generated with it in Node.
var lineDiffCases = map[string]struct {
	old, new string
	hunks    []Hunk
	gaps     []Gap
}{
	"edited": {"a\nb\nc\nd\ne\n", "a\nB\nX\nc\nd\ne\n", []Hunk{
		{1, 5, 1, 6, []Block{{Context, 1, 1, 1, 1}, {Change, 2, 1, 2, 2}, {Context, 3, 3, 4, 3}}}}, []Gap{}},
	"shrunk": {"a\nb\nc\nd\n", "a\nX\nd\n", []Hunk{
		{1, 4, 1, 3, []Block{{Context, 1, 1, 1, 1}, {Change, 2, 2, 2, 1}, {Context, 4, 1, 3, 1}}}}, []Gap{}},
	"long": {"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\ninserted\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", []Hunk{
		{16, 8, 16, 9, []Block{{Context, 16, 4, 16, 4}, {Change, 20, 0, 20, 1}, {Context, 20, 4, 21, 4}}}}, []Gap{{0, 1, 1, 15}, {1, 24, 25, 17}}},
	"added": {"", "a\nb\n", []Hunk{
		{1, 0, 1, 2, []Block{{Change, 1, 0, 1, 2}}}}, []Gap{}},
	"deleted": {"a\nb\n", "", []Hunk{
		{1, 2, 1, 0, []Block{{Change, 1, 2, 1, 0}}}}, []Gap{}},
	"hunks": {"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nten\nTEN\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 31\nline 32\nline 33\nline 34\nline 35\nline 36\nline 37\nline 38\nline 39\nline 40\n", []Hunk{
		{6, 9, 6, 10, []Block{{Context, 6, 4, 6, 4}, {Change, 10, 1, 10, 2}, {Context, 11, 4, 12, 4}}},
		{26, 9, 27, 8, []Block{{Context, 26, 4, 27, 4}, {Change, 30, 1, 31, 0}, {Context, 31, 4, 31, 4}}}}, []Gap{{0, 1, 1, 5}, {1, 15, 16, 11}, {2, 35, 35, 6}}},
	"tail": {"line 1\nline 2\nline 3\nline 4\nline 5\n", "line 1\nline 2\nline 3\n", []Hunk{
		{1, 5, 1, 3, []Block{{Context, 1, 3, 1, 3}, {Change, 4, 2, 4, 0}}}}, []Gap{}},
	"deleted3": {"line 1\nline 2\nline 3\n", "", []Hunk{
		{1, 3, 1, 0, []Block{{Change, 1, 3, 1, 0}}}}, []Gap{}},
	"added3": {"", "line 1\nline 2\nline 3\n", []Hunk{
		{1, 0, 1, 3, []Block{{Change, 1, 0, 1, 3}}}}, []Gap{}},
	"head": {"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\n", "A\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\n", []Hunk{
		{1, 5, 1, 5, []Block{{Change, 1, 1, 1, 1}, {Context, 2, 4, 2, 4}}}}, []Gap{{1, 6, 6, 3}}},
}

// diffOf is a case as a Diff.
func diffOf(name string) *Diff {
	c := lineDiffCases[name]
	return &Diff{Old: SplitLines(c.old), New: SplitLines(c.new), Hunks: c.hunks, Gaps: c.gaps}
}
