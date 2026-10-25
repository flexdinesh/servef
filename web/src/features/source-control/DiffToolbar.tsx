import { Menu } from "@base-ui/react/menu"
import { CheckIcon, ChevronDownIcon, ChevronUpIcon, ChevronsDownUpIcon, ChevronsUpDownIcon, Columns2Icon, HighlighterIcon, PanelRightIcon, Rows2Icon, SpaceIcon, UnfoldVerticalIcon, WrapTextIcon } from "lucide-react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { menuItemClass, menuPopupClass, menuPositionerClass } from "../../components/ui/menu.ts"
import { SegmentedControl, SegmentedItem } from "../../components/ui/segmented-control.tsx"
import { effectiveDiffStyle, inlineChoices, type DiffOptions } from "./diff-options.ts"

interface DiffToolbarProps {
  options: DiffOptions
  narrow: boolean
  // hasChanges: the review has files whose changes steps can reach.
  hasChanges: boolean
  // anyExpanded: some file's diff is open, so the action collapses all.
  anyExpanded: boolean
  onChange(change: Partial<DiffOptions>): void
  onExpandAll(expand: boolean): void
  onStepHunk(direction: 1 | -1): void
  // files toggles the review's Files rail, or opens its sheet on phones;
  // absent for reviews without one.
  files?: { shown: boolean; onToggle(): void }
}

const splitUnavailable = "Split view needs a wider window"
const noChanges = "No changes to step through"

// DiffToolbar sits in the document mode-toggle slot for a review tab.
export function DiffToolbar({ anyExpanded, files, hasChanges, narrow, onChange, onExpandAll, onStepHunk, options }: DiffToolbarProps) {
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
      <IconButton className="diff-open-file" label={anyExpanded ? "Collapse All Files" : "Expand All Files"}
        icon={anyExpanded ? <ChevronsDownUpIcon aria-hidden="true" /> : <ChevronsUpDownIcon aria-hidden="true" />}
        onClick={() => onExpandAll(!anyExpanded)} />
      {files && (
        <IconButton className="diff-files-toggle" label="Files" title={files.shown ? "Hide Files" : "Show Files"} aria-pressed={files.shown}
          icon={<PanelRightIcon aria-hidden="true" />} onClick={files.onToggle} />
      )}
    </div>
  )
}
