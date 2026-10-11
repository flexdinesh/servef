import { Menu } from "@base-ui/react/menu"
import { CheckIcon, ChevronDownIcon } from "lucide-react"
import { useLayoutEffect, useRef, useState } from "react"

import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { fileTypeCounts } from "./file-type-filter.ts"
import type { FileType, TreeNode } from "./page-contract.ts"

interface FileTypeFilterProps {
  fileTypes: readonly FileType[]
  tree: readonly TreeNode[]
  value: readonly string[]
  onChange(value: readonly string[]): void
}

export function FileTypeFilter({ fileTypes, tree, value, onChange }: FileTypeFilterProps) {
  const trigger = useRef<HTMLButtonElement>(null)
  const [container, setContainer] = useState<HTMLElement | null>(null)
  useLayoutEffect(() => { setContainer(trigger.current?.closest("dialog") ?? null) }, [])
  const counts = fileTypeCounts(tree)
  const selectedTypes = fileTypes.filter((type) => value.includes(type.id))
  const visibleTypes = selectedTypes.length > 0 ? selectedTypes : fileTypes
  const count = visibleTypes.reduce((total, type) => total + (counts.get(type.id) ?? 0), 0)
  const allVisible = selectedTypes.length === 0 || selectedTypes.length === fileTypes.length
  const onlyType = fileTypes.length === 1 ? fileTypes[0] : undefined
  const label = onlyType?.label ?? (allVisible ? "All types"
    : selectedTypes.length === 1 ? selectedTypes[0]?.label
    : `${selectedTypes.length} types`)

  return <div className="file-type-filter">
    <DropdownMenu>
      <DropdownMenuTrigger ref={trigger} className="file-type-filter-trigger" aria-label="Filter file types"
        disabled={fileTypes.length < 2} title={onlyType ? `${onlyType.label} only · set by CLI`
          : selectedTypes.length > 1 && !allVisible ? selectedTypes.map((type) => type.label).join(", ") : "Filter files by type"}>
        <span>{label} · {count}</span>
        {onlyType ? <span className="file-type-filter-scope">Set by CLI</span> : <ChevronDownIcon aria-hidden="true" />}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="file-type-filter-menu" aria-label="File types"
        container={container ?? undefined}>
        <Menu.Item className="file-type-filter-action" closeOnClick={false} disabled={selectedTypes.length === 0}
          onClick={() => onChange([])}>Show all types</Menu.Item>
        <Menu.Separator className="file-type-filter-separator" />
        {fileTypes.map((type) => <Menu.CheckboxItem key={type.id} className="file-type-filter-option"
          checked={value.includes(type.id)} closeOnClick={false}
          onCheckedChange={(checked) => {
            const next = fileTypes.filter((entry) => entry.id === type.id ? checked : value.includes(entry.id))
              .map((entry) => entry.id)
            onChange(next)
          }}>
          <span className="file-type-filter-checkbox"><Menu.CheckboxItemIndicator><CheckIcon aria-hidden="true" /></Menu.CheckboxItemIndicator></span>
          <span>{type.label}</span><span className="file-type-filter-count">{counts.get(type.id) ?? 0}</span>
        </Menu.CheckboxItem>)}
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
}
