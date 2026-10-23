import { Menu } from "@base-ui/react/menu"
import { CheckIcon, ChevronDownIcon, ChevronUpIcon, Columns2Icon, FileInputIcon, HighlighterIcon, Rows2Icon, SpaceIcon, UnfoldVerticalIcon, WrapTextIcon } from "lucide-react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { menuItemClass, menuPopupClass, menuPositionerClass } from "../../components/ui/menu.ts"
import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import { effectiveDiffStyle, inlineChoices, type DiffOptions } from "./diff-options.ts"

interface DiffToolbarProps {
  options: DiffOptions
  narrow: boolean
  // openFileReason explains why Open File is unavailable, else "".
  openFileReason: string
  // hasChanges: the shown diff has hunks to step through.
  hasChanges: boolean
  onChange(change: Partial<DiffOptions>): void
  onOpenFile(): void
  onStepHunk(direction: 1 | -1): void
}

const splitUnavailable = "Split view needs a wider window"
const noChanges = "No changes to step through"

// DiffToolbar sits in the document mode-toggle slot for a diff tab.
export function DiffToolbar({ hasChanges, narrow, onChange, onOpenFile, onStepHunk, openFileReason, options }: DiffToolbarProps) {
  const style = effectiveDiffStyle(options, narrow)
  const inline = inlineChoices.find((choice) => choice.value === options.inline) ?? inlineChoices[0]
  return (
    <div className="diff-toolbar">
      <SegmentedControl className="document-mode-toggle diff-layout" aria-label="Diff layout">
        <SegmentedItem aria-label="Split" pressed={style === "split"} aria-disabled={narrow || undefined}
          title={narrow ? splitUnavailable : "Split"} onClick={() => { if (!narrow) onChange({ style: "split" }) }}>
          <Columns2Icon aria-hidden="true" />
        </SegmentedItem>
        <SegmentedItem aria-label="Unified" pressed={style === "unified"} title="Unified"
          onClick={() => onChange({ style: "unified" })}>
          <Rows2Icon aria-hidden="true" />
        </SegmentedItem>
      </SegmentedControl>
      <SegmentedControl className="document-mode-toggle" aria-label="Diff options">
        <SegmentedItem aria-label="Wrap lines" pressed={options.wrap} title="Wrap lines"
          onClick={() => onChange({ wrap: !options.wrap })}>
          <WrapTextIcon aria-hidden="true" />
        </SegmentedItem>
        <SegmentedItem aria-label="Ignore whitespace" pressed={options.ignoreWhitespace}
          title="Ignore leading and trailing whitespace" onClick={() => onChange({ ignoreWhitespace: !options.ignoreWhitespace })}>
          <SpaceIcon aria-hidden="true" />
        </SegmentedItem>
        <Menu.Root>
          <Menu.Trigger className="diff-inline-trigger" aria-label={`Inline changes: ${inline?.label ?? "Word"}`}
            title={`Inline changes: ${inline?.label ?? "Word"}`} data-active={options.inline !== "none" || undefined}
            render={<SegmentedItem />}>
            <HighlighterIcon aria-hidden="true" />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner className={menuPositionerClass} sideOffset={4} align="end">
              <Menu.Popup className={menuPopupClass} aria-label="Inline changes">
                <Menu.RadioGroup value={options.inline} onValueChange={(value: unknown) => {
                  const choice = inlineChoices.find((entry) => entry.value === value)
                  if (choice) onChange({ inline: choice.value })
                }}>
                  {inlineChoices.map((choice) => (
                    <Menu.RadioItem key={choice.value} className={menuItemClass} value={choice.value} closeOnClick>
                      <Menu.RadioItemIndicator className="diff-inline-check" keepMounted>
                        <CheckIcon aria-hidden="true" />
                      </Menu.RadioItemIndicator>
                      {choice.label}
                    </Menu.RadioItem>
                  ))}
                </Menu.RadioGroup>
              </Menu.Popup>
            </Menu.Positioner>
          </Menu.Portal>
        </Menu.Root>
        <SegmentedItem aria-label="Expand unchanged lines" pressed={options.expandUnchanged}
          title="Expand unchanged lines" onClick={() => onChange({ expandUnchanged: !options.expandUnchanged })}>
          <UnfoldVerticalIcon aria-hidden="true" />
        </SegmentedItem>
      </SegmentedControl>
      <span className="diff-hunk-steps">
        <IconButton label="Previous change" title="Previous change (Shift+F7)" aria-keyshortcuts="Shift+F7"
          icon={<ChevronUpIcon aria-hidden="true" />} disabledReason={hasChanges ? undefined : noChanges}
          onClick={() => onStepHunk(-1)} />
        <IconButton label="Next change" title="Next change (F7)" aria-keyshortcuts="F7"
          icon={<ChevronDownIcon aria-hidden="true" />} disabledReason={hasChanges ? undefined : noChanges}
          onClick={() => onStepHunk(1)} />
      </span>
      <IconButton className="diff-open-file" label="Open File" icon={<FileInputIcon aria-hidden="true" />}
        disabledReason={openFileReason || undefined} onClick={onOpenFile} />
    </div>
  )
}
