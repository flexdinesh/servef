import { Menu } from "@base-ui/react/menu"
import { CheckIcon, Columns2Icon, FileInputIcon, HighlighterIcon, Rows2Icon, SpaceIcon, UnfoldVerticalIcon, WrapTextIcon } from "lucide-react"

import { effectiveDiffStyle, inlineChoices, type DiffOptions } from "./diff-options.ts"
import "../../tab-menu.css"

interface DiffToolbarProps {
  options: DiffOptions
  narrow: boolean
  // openFileReason explains why Open File is unavailable, else "".
  openFileReason: string
  onChange(change: Partial<DiffOptions>): void
  onOpenFile(): void
}

const splitUnavailable = "Split view needs a wider window"

// DiffToolbar sits in the document mode-toggle slot for a diff tab.
export function DiffToolbar({ narrow, onChange, onOpenFile, openFileReason, options }: DiffToolbarProps) {
  const style = effectiveDiffStyle(options, narrow)
  const inline = inlineChoices.find((choice) => choice.value === options.inline) ?? inlineChoices[0]
  return (
    <div className="diff-toolbar">
      <div className="document-mode-toggle diff-layout" role="group" aria-label="Diff layout">
        <button type="button" aria-label="Split" aria-pressed={style === "split"} aria-disabled={narrow || undefined}
          title={narrow ? splitUnavailable : "Split"} onClick={() => { if (!narrow) onChange({ style: "split" }) }}>
          <Columns2Icon aria-hidden="true" />
        </button>
        <button type="button" aria-label="Unified" aria-pressed={style === "unified"} title="Unified"
          onClick={() => onChange({ style: "unified" })}>
          <Rows2Icon aria-hidden="true" />
        </button>
      </div>
      <div className="document-mode-toggle" role="group" aria-label="Diff options">
        <button type="button" aria-label="Wrap lines" aria-pressed={options.wrap} title="Wrap lines"
          onClick={() => onChange({ wrap: !options.wrap })}>
          <WrapTextIcon aria-hidden="true" />
        </button>
        <button type="button" aria-label="Ignore whitespace" aria-pressed={options.ignoreWhitespace}
          title="Ignore leading and trailing whitespace" onClick={() => onChange({ ignoreWhitespace: !options.ignoreWhitespace })}>
          <SpaceIcon aria-hidden="true" />
        </button>
        <Menu.Root>
          <Menu.Trigger className="diff-inline-trigger" aria-label={`Inline changes: ${inline?.label ?? "Word"}`}
            title={`Inline changes: ${inline?.label ?? "Word"}`} data-active={options.inline !== "none" || undefined}>
            <HighlighterIcon aria-hidden="true" />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Positioner className="tab-menu-positioner" sideOffset={4} align="end">
              <Menu.Popup className="tab-menu diff-inline-menu" aria-label="Inline changes">
                <Menu.RadioGroup value={options.inline} onValueChange={(value: unknown) => {
                  const choice = inlineChoices.find((entry) => entry.value === value)
                  if (choice) onChange({ inline: choice.value })
                }}>
                  {inlineChoices.map((choice) => (
                    <Menu.RadioItem key={choice.value} className="tab-menu-item diff-inline-item" value={choice.value} closeOnClick>
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
        <button type="button" aria-label="Expand unchanged lines" aria-pressed={options.expandUnchanged}
          title="Expand unchanged lines" onClick={() => onChange({ expandUnchanged: !options.expandUnchanged })}>
          <UnfoldVerticalIcon aria-hidden="true" />
        </button>
      </div>
      <button type="button" className="files-toggle diff-open-file" aria-label="Open File" aria-disabled={openFileReason ? true : undefined}
        title={openFileReason || "Open File"} onClick={() => { if (!openFileReason) onOpenFile() }}>
        <FileInputIcon aria-hidden="true" />
      </button>
    </div>
  )
}
