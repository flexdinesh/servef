import { fileTypeCounts } from "./file-type-filter.ts"
import type { FileType, TreeNode } from "./page-contract.ts"

interface FileTypeFilterProps {
  fileTypes: readonly FileType[]
  tree: readonly TreeNode[]
  value: string
  onChange(value: string): void
}

export function FileTypeFilter({ fileTypes, tree, value, onChange }: FileTypeFilterProps) {
  const counts = fileTypeCounts(tree)
  const onlyType = fileTypes.length === 1 ? fileTypes[0] : undefined
  return <div className="file-type-filter">
    <select aria-label="Filter file types" value={onlyType?.id ?? value}
      disabled={fileTypes.length < 2}
      title={onlyType ? `${onlyType.label} only · set by CLI` : "Filter files by type"}
      onChange={(event) => onChange(event.target.value)}>
      {!onlyType && <option value="">All types · {[...counts.values()].reduce((total, count) => total + count, 0)}</option>}
      {fileTypes.map((fileType) => <option key={fileType.id} value={fileType.id}>
        {fileType.label} · {counts.get(fileType.id) ?? 0}
      </option>)}
    </select>
    {onlyType && <span>Set by CLI</span>}
  </div>
}
