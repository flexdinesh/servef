// Pierre trees take their row height in px. Explorer and Changes share it so
// their rows match; scale follows the Font Size setting.
const desktopRow = 24
const narrowRow = 32

export function treeRowHeight(narrow: boolean, scale: number): number {
  return Math.round((narrow ? narrowRow : desktopRow) * scale)
}
