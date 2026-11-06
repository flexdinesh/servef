import { DEFAULT_VIRTUAL_FILE_METRICS, type VirtualFileMetrics } from "@pierre/diffs"

// Diff rows are 1.375rem tall (diff.css `--diffs-line-height`); a rem is 16px
// at the default Font Size, so rows scale with the setting. Pierre's hunk
// separators and padding stay px.
const rowRem = 1.375
const remPx = 16

// diffMetrics is the virtualizer's row estimate at a UI scale; Pierre measures
// wrapped rows itself. Spacing matches diff.css `--diffs-gap-block`, which runs
// code and collapsed-line bands flush.
export function diffMetrics(scale: number): VirtualFileMetrics {
  return { ...DEFAULT_VIRTUAL_FILE_METRICS, lineHeight: rowRem * remPx * scale, spacing: 0 }
}
