import { CaseSensitiveIcon, FilterIcon, RefreshCwIcon, SearchIcon, TriangleAlertIcon, XIcon } from "lucide-react"
import { useRef, useState, type ReactNode } from "react"

import { Badge } from "../components/ui/badge.tsx"
import { Button, type ButtonSize, type ButtonVariant } from "../components/ui/button.tsx"
import { CopyButton } from "../components/ui/copy-button.tsx"
import { DialogShell } from "../components/ui/dialog-shell.tsx"
import { IconButton, type IconButtonSize } from "../components/ui/icon-button.tsx"
import { Kbd } from "../components/ui/kbd.tsx"
import { ListButton, ListRow } from "../components/ui/list-row.tsx"
import { menuItemClass, menuPopupClass, menuSeparatorClass } from "../components/ui/menu.ts"
import { Notice, StatusMessage } from "../components/ui/notice.tsx"
import { ProgressBar } from "../components/ui/progress-bar.tsx"
import { Chevron, SectionToggle } from "../components/ui/section-toggle.tsx"
import { SegmentedControl, SegmentedItem } from "../components/ui/segmented-control.tsx"
import { Select } from "../components/ui/select.tsx"
import { TextField } from "../components/ui/text-field.tsx"
import { ToggleButton } from "../components/ui/toggle-button.tsx"
import { ViewHeader } from "../components/ui/view-header.tsx"
import { useTheme } from "../settings/ThemeProvider.tsx"
import { isTheme, themeOptions } from "../settings/theme.ts"
import "./gallery.css"

const buttonVariants: readonly ButtonVariant[] = ["primary", "outline", "ghost", "destructive", "link"]
const buttonSizes: readonly ButtonSize[] = ["sm", "md", "lg"]
const iconSizes: readonly IconButtonSize[] = ["xs", "sm", "md", "bar"]
const segmentSizes: readonly ("sm" | "md")[] = ["md", "sm"]

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="gallery-section" aria-label={title}>
      <h2>{title}</h2>
      {children}
    </section>
  )
}

function Row({ children }: { children: ReactNode }) {
  return <div className="gallery-row">{children}</div>
}

// UiGallery renders every primitive in its states. Dev-only: served at /__ui
// in dev:mock and absent from production bundles.
export default function UiGallery() {
  const { theme, setTheme } = useTheme()
  const [mode, setMode] = useState("preview")
  const [caseSensitive, setCaseSensitive] = useState(true)
  const [expanded, setExpanded] = useState(true)
  const [busy, setBusy] = useState(true)
  const dialog = useRef<HTMLDialogElement>(null)

  return (
    <main className="gallery">
      <header className="gallery-header">
        <h1>servef UI primitives</h1>
        <Select aria-label="Theme" value={theme} onChange={(event) => { if (isTheme(event.target.value)) setTheme(event.target.value) }}>
          {themeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </Select>
      </header>

      <Section title="Button">
        {buttonVariants.map((variant) => (
          <Row key={variant}>
            {buttonSizes.map((size) => <Button key={size} variant={variant} size={size}>{variant} {size}</Button>)}
            <Button variant={variant} disabled>disabled</Button>
            <Button variant={variant}><RefreshCwIcon aria-hidden="true" />with icon</Button>
          </Row>
        ))}
      </Section>

      <Section title="IconButton">
        <Row>
          {iconSizes.map((size) => <IconButton key={size} size={size} label={`Refresh ${size}`} icon={<RefreshCwIcon />} />)}
          <IconButton label="Close" tone="destructive" icon={<XIcon />} />
          <IconButton label="Refresh" disabledReason="Nothing to refresh" icon={<RefreshCwIcon />} />
          <IconButton label="Disabled" disabled icon={<RefreshCwIcon />} />
        </Row>
      </Section>

      <Section title="ToggleButton and SegmentedControl">
        <Row>
          <ToggleButton label="Match case" pressed={caseSensitive} onPressedChange={setCaseSensitive}><CaseSensitiveIcon /></ToggleButton>
          <ToggleButton label="Match case off" pressed={false} onPressedChange={() => undefined}><CaseSensitiveIcon /></ToggleButton>
          <ToggleButton size="text" pressed={!caseSensitive} onPressedChange={(next) => setCaseSensitive(!next)}>Show modified</ToggleButton>
        </Row>
        <Row>
          {segmentSizes.map((size) => (
            <SegmentedControl key={size} size={size} aria-label={`Mode ${size}`}>
              {["preview", "source", "format"].map((item) => (
                <SegmentedItem key={item} pressed={mode === item} onClick={() => setMode(item)}>{item}</SegmentedItem>
              ))}
              <SegmentedItem disabled>disabled</SegmentedItem>
            </SegmentedControl>
          ))}
        </Row>
      </Section>

      <Section title="TextField and Select">
        <Row>
          <TextField aria-label="Filter" placeholder="Filter files" icon={<FilterIcon />} trailing={<span>12</span>} />
          <TextField aria-label="Search" placeholder="Search" trailing={<ToggleButton label="Match case" pressed={caseSensitive} onPressedChange={setCaseSensitive}><CaseSensitiveIcon /></ToggleButton>} />
          <TextField aria-label="Invalid" defaultValue="[unclosed" invalid />
          <Select aria-label="Select"><option>Option one</option><option>Option two</option></Select>
          <Select aria-label="Disabled select" disabled><option>Disabled</option></Select>
        </Row>
        <div className="gallery-palette">
          <TextField variant="palette" aria-label="Palette" placeholder="Go to file" icon={<SearchIcon />} trailing={<Kbd>Esc</Kbd>} />
        </div>
      </Section>

      <Section title="ListRow, SectionToggle, Menu">
        <div className="gallery-list">
          <SectionToggle expanded={expanded} onExpandedChange={setExpanded}>Changes <Badge>3</Badge></SectionToggle>
          {expanded && (
            <>
              <ListRow tabIndex={0}>Rest row</ListRow>
              <ListRow tabIndex={0} active>Active row</ListRow>
              <ListRow tabIndex={0} selected>Selected row</ListRow>
              <ListRow tabIndex={0} aria-expanded="false"><Chevron />Collapsed folder</ListRow>
              <ListRow tabIndex={0} density="pick">Pick density</ListRow>
              <ListButton selected>Settings category</ListButton>
              <ListButton>Another category</ListButton>
            </>
          )}
        </div>
        <div className={menuPopupClass} role="menu" aria-label="Menu sample">
          <div className={menuItemClass} role="menuitem">Close</div>
          <div className={menuItemClass} role="menuitem" data-highlighted="">Close others</div>
          <div className={menuSeparatorClass} role="separator" />
          <div className={menuItemClass} role="menuitem" data-disabled="">Close to the right</div>
        </div>
      </Section>

      <Section title="Badge, Kbd, CopyButton">
        <Row>
          <Badge>12</Badge>
          <Badge tone="accent">3</Badge>
          <Kbd>Ctrl P</Kbd>
          <CopyButton text="plain" label="Copy path" />
          <CopyButton text="raised" label="Copy code" variant="raised" />
          <CopyButton text="chip" label="Copy typescript code" variant="chip">typescript</CopyButton>
        </Row>
      </Section>

      <Section title="Notice, StatusMessage, ProgressBar">
        <Notice tone="info">Search results are limited to 2,000 matches.</Notice>
        <Notice tone="warning"><TriangleAlertIcon aria-hidden="true" />The language server is starting.</Notice>
        <Notice tone="error">Could not read the file.</Notice>
        <Notice tone="warning" layout="bar">History is shallow.</Notice>
        <StatusMessage>No results</StatusMessage>
        <StatusMessage tone="error">Search failed</StatusMessage>
        <Row>
          <Button variant="outline" size="sm" onClick={() => setBusy(!busy)}>Toggle progress</Button>
        </Row>
        <div className="gallery-progress"><ProgressBar active={busy} /></div>
      </Section>

      <Section title="ViewHeader and DialogShell">
        <div className="gallery-list">
          <ViewHeader title="Source Control" actions={<><IconButton label="Refresh" icon={<RefreshCwIcon />} /><IconButton label="Filter" icon={<FilterIcon />} /></>} />
        </div>
        <Row>
          <Button variant="outline" onClick={() => dialog.current?.showModal()}>Open dialog</Button>
        </Row>
        <DialogShell ref={dialog} variant="panel" aria-label="Sample dialog">
          <ViewHeader variant="dialog" title="Settings" actions={<IconButton label="Close" icon={<XIcon />} onClick={() => dialog.current?.close()} />} />
          <div className="gallery-dialog-body">
            <StatusMessage>Dialog body</StatusMessage>
            <Button onClick={() => dialog.current?.close()}>Done</Button>
          </div>
        </DialogShell>
      </Section>
    </main>
  )
}
