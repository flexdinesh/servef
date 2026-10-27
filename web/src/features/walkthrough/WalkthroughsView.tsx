import {
  ArrowLeftIcon, BookOpenIcon, CheckIcon, CopyIcon, FileClockIcon, FileCodeIcon, GitCompareArrowsIcon, PresentationIcon, RefreshCwIcon, RouteIcon, StickyNoteIcon,
  Trash2Icon, XIcon,
} from "lucide-react"
import { useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react"

import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ListRow } from "../../components/ui/list-row.tsx"
import { Notice, StatusMessage } from "../../components/ui/notice.tsx"
import { ViewHeader } from "../../components/ui/view-header.tsx"
import type { Walkthrough, WalkthroughStep } from "../../api/walkthrough-contract.ts"
import { useClipboardCopy } from "../../lib/clipboard.ts"
import { commitAge, commitDate, type CompareContext } from "../source-control/compare.ts"
import { agentInstructions } from "./agent-instructions.ts"
import type { WalkthroughSession } from "./use-walkthrough-session.ts"
import { stepLabel, stepTarget, type StepPosition } from "./walkthrough-steps.ts"

interface WalkthroughsViewProps {
  // project is the absolute path the copied agent instructions publish to.
  project: string
  session: WalkthroughSession
  context: CompareContext
  mobile: boolean
  onDismiss(): void
  // onPresent follows the walkthrough with the workspace chrome hidden.
  onPresent(): void
}

function plural(count: number, noun: string): string {
  return `${count.toLocaleString()} ${noun}${count === 1 ? "" : "s"}`
}

function stepIcon(step: WalkthroughStep): ReactNode {
  const target = stepTarget(step)
  if (target.kind === "file") return target.at ? <FileClockIcon aria-hidden="true" /> : <FileCodeIcon aria-hidden="true" />
  if (target.kind === "diff") return <GitCompareArrowsIcon aria-hidden="true" />
  return <StickyNoteIcon aria-hidden="true" />
}

// useRovingRows moves focus through a listbox's options with the arrow keys,
// Home, and End; Enter activates the focused one.
function useRovingRows(count: number, current: number, activate: (index: number) => void) {
  const container = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(-1)
  const tabIndex = focused >= 0 && focused < count ? focused : Math.max(0, current)
  function focusRow(index: number) {
    setFocused(index)
    container.current?.querySelectorAll<HTMLElement>("[role=option]")[index]?.focus()
  }
  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      const next = tabIndex + (event.key === "ArrowDown" ? 1 : -1)
      if (next >= 0 && next < count) focusRow(next)
    } else if (event.key === "Home" || event.key === "End") {
      focusRow(event.key === "Home" ? 0 : count - 1)
    } else if (event.key === "Enter" || event.key === " ") {
      activate(tabIndex)
    } else {
      return
    }
    event.preventDefault()
  }
  return { container, tabIndex, setFocused, onKeyDown }
}

// WalkthroughsView is the Walkthroughs activity (decision 0047): the
// project's walkthroughs, newest first, then one walkthrough's steps.
// Choosing a step opens its file or review with the narration in place.
export function WalkthroughsView({ context, mobile, onDismiss, onPresent, project, session }: WalkthroughsViewProps) {
  const copy = <CopyInstructions project={project} />
  const close = mobile ? <IconButton label="Close walkthroughs" icon={<XIcon aria-hidden="true" />} onClick={onDismiss} /> : null
  if (!session.selectedId) {
    return (
      <div className="walkthroughs-view">
        <ViewHeader className="explorer-header sidebar-header" title="Walkthroughs" actions={<>
          <IconButton label="Refresh" icon={<RefreshCwIcon aria-hidden="true" />} onClick={session.list.refresh} />
          {copy}
          {close}
        </>} />
        <WalkthroughList session={session} project={project} onOpened={mobile ? onDismiss : undefined} />
      </div>
    )
  }
  return (
    <div className="walkthroughs-view">
      <ViewHeader className="explorer-header sidebar-header" title="Walkthroughs" actions={<>
        <IconButton label="All Walkthroughs" icon={<ArrowLeftIcon aria-hidden="true" />} onClick={session.close} />
        <IconButton label="Present" icon={<PresentationIcon aria-hidden="true" />}
          disabledReason={session.walkthrough ? undefined : "Waiting for the walkthrough"}
          onClick={() => {
            onPresent()
            if (mobile) onDismiss()
          }} />
        {copy}
        {close}
      </>} />
      <WalkthroughDetailView session={session} context={context} onOpened={mobile ? onDismiss : undefined} />
    </div>
  )
}

// CopyInstructions copies the prompt that teaches an agent to prepare and
// publish a walkthrough to this servef (decision 0049): an icon in the view
// header, or a labeled button in the empty list.
function CopyInstructions({ project, labeled = false }: { project: string; labeled?: boolean }) {
  const { status, copy } = useClipboardCopy()
  const onClick = () => { void copy(agentInstructions(window.location.origin, project)) }
  const icon = status === "copied" ? <CheckIcon aria-hidden="true" /> : status === "failed" ? <XIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />
  return (
    <>
      {labeled
        ? <Button variant="outline" data-status={status} onClick={onClick}>
          {icon}{status === "copied" ? "Copied" : status === "failed" ? "Copy failed" : "Copy agent instructions"}
        </Button>
        : <IconButton label="Copy Agent Instructions" title={status === "failed" ? "Copy failed" : undefined}
          tone={status === "copied" ? "success" : status === "failed" ? "destructive" : "default"} data-status={status} icon={icon} onClick={onClick} />}
      <span className="visually-hidden" role="status">{status === "copied" ? "Agent instructions copied" : status === "failed" ? "Copy failed" : ""}</span>
    </>
  )
}

interface ListProps {
  session: WalkthroughSession
  project: string
  onOpened: (() => void) | undefined
}

function WalkthroughList({ session, project, onOpened }: ListProps) {
  const { error, refresh, result } = session.list
  const entries = useMemo(() => result ?? [], [result])
  const now = useMemo(() => result ? Date.now() : 0, [result])
  const open = (index: number) => {
    const entry = entries[index]
    if (!entry) return
    session.open(entry.id)
    onOpened?.()
  }
  const { container, tabIndex, setFocused, onKeyDown } = useRovingRows(entries.length, -1, open)
  if (error && !result) {
    return (
      <div className="walkthroughs-message">
        <StatusMessage tone="error" role="alert">{error}</StatusMessage>
        <Button variant="outline" onClick={refresh}>Retry</Button>
      </div>
    )
  }
  if (!result) return <div className="walkthroughs-message"><StatusMessage>Loading walkthroughs…</StatusMessage></div>
  if (entries.length === 0) {
    return (
      <div className="walkthroughs-message">
        <StatusMessage>No walkthroughs for this project yet.</StatusMessage>
        <StatusMessage>Ask an agent to explain a change or feature as a walkthrough. Paste these instructions into it first.</StatusMessage>
        <CopyInstructions project={project} labeled />
      </div>
    )
  }
  return (
    <div className="walkthroughs-scroll">
      {error && <StatusMessage tone="error" role="alert" className="walkthroughs-stale">{error}</StatusMessage>}
      <div ref={container} className="walkthrough-rows" role="listbox" aria-label="Walkthroughs" onKeyDown={onKeyDown}>
        {entries.map((entry, index) => {
          const age = commitAge(entry.updatedAt, now)
          return (
            <ListRow key={entry.id} role="option" aria-selected={false} tabIndex={index === tabIndex ? 0 : -1}
              className="walkthrough-row" title={`${entry.title}\nUpdated ${commitDate(entry.updatedAt)}`}
              aria-label={`${entry.title}, ${plural(entry.stepCount, "step")}, updated ${age.long}`}
              onFocus={() => setFocused(index)} onClick={() => open(index)}>
              <RouteIcon className="walkthrough-row-icon" aria-hidden="true" />
              <span className="walkthrough-row-text">
                <span className="walkthrough-row-title">{entry.title}</span>
                <span className="walkthrough-row-meta">{plural(entry.stepCount, "step")} · {age.long}</span>
              </span>
            </ListRow>
          )
        })}
      </div>
    </div>
  )
}

interface DetailProps {
  session: WalkthroughSession
  context: CompareContext
  onOpened: (() => void) | undefined
}

function WalkthroughDetailView({ context, session, onOpened }: DetailProps) {
  const { detail } = session
  if (detail.kind === "missing") {
    return (
      <div className="walkthroughs-message">
        <StatusMessage>This walkthrough was deleted.</StatusMessage>
        <Button variant="outline" onClick={session.close}>All Walkthroughs</Button>
      </div>
    )
  }
  if (detail.kind === "error") {
    return (
      <div className="walkthroughs-message">
        <StatusMessage tone="error" role="alert">{detail.message}</StatusMessage>
        <Button variant="outline" onClick={session.retry}>Retry</Button>
      </div>
    )
  }
  if (detail.kind !== "ready") return <div className="walkthroughs-message"><StatusMessage>Loading walkthrough…</StatusMessage></div>
  return <StepList key={detail.walkthrough.id} walkthrough={detail.walkthrough} context={context} session={session} onOpened={onOpened} />
}

interface StepListProps extends DetailProps {
  walkthrough: Walkthrough
}

// Options are the overview (the cover) and then each step.
function positionAt(index: number): StepPosition {
  return index === 0 ? null : index - 1
}

function StepList({ context, onOpened, session, walkthrough }: StepListProps) {
  const [confirming, setConfirming] = useState(false)
  const [deleteError, setDeleteError] = useState("")
  const { position, routed, visited } = session
  const current = position === null ? 0 : position + 1
  const open = (index: number) => {
    session.goTo(positionAt(index), false)
    onOpened?.()
  }
  const { container, tabIndex, setFocused, onKeyDown } = useRovingRows(walkthrough.steps.length + 1, current, open)
  const now = useMemo(() => walkthrough.updatedAt ? Date.now() : 0, [walkthrough.updatedAt])
  const remove = () => {
    setDeleteError("")
    session.list.remove(walkthrough.id).then(session.close, (error: unknown) => {
      setDeleteError(error instanceof Error ? error.message : "Couldn't delete the walkthrough.")
    })
  }
  return (
    <div className="walkthroughs-scroll">
      <div className="walkthrough-heading">
        <h3 className="walkthrough-title">{walkthrough.title}</h3>
        <p className="walkthrough-meta" title={`Updated ${commitDate(walkthrough.updatedAt)}`}>
          {position === null ? plural(walkthrough.steps.length, "step") : `Step ${position + 1} of ${walkthrough.steps.length}`}
          {" · "}{commitAge(walkthrough.updatedAt, now).long}
        </p>
        {!confirming && (
          <IconButton className="walkthrough-delete" label="Delete Walkthrough" icon={<Trash2Icon aria-hidden="true" />}
            onClick={() => setConfirming(true)} />
        )}
      </div>
      {confirming && (
        <Notice tone="warning" layout="bar" role="group" aria-label="Delete walkthrough" className="walkthrough-confirm">
          <span>Delete this walkthrough? servef keeps no copy.</span>
          {deleteError && <span role="alert">{deleteError}</span>}
          <span className="walkthrough-confirm-actions">
            <Button size="sm" variant="destructive" onClick={remove}>Delete</Button>
            <Button size="sm" variant="outline" onClick={() => setConfirming(false)}>Cancel</Button>
          </span>
        </Notice>
      )}
      <div ref={container} className="walkthrough-rows" role="listbox" aria-label="Steps" onKeyDown={onKeyDown}>
        {[null, ...walkthrough.steps].map((step, index) => {
          const selected = routed && index === current
          const label = step ? stepLabel(step, context) : "Overview"
          const title = step ? step.title : "Overview"
          return (
            <ListRow key={index} role="option" aria-selected={selected} selected={selected}
              tabIndex={index === tabIndex ? 0 : -1} className="walkthrough-step"
              data-visited={(step && visited.has(index - 1)) || undefined}
              aria-label={step ? `Step ${index} of ${walkthrough.steps.length}: ${title}, ${label}` : "Overview"}
              title={step ? `${title}\n${label}` : walkthrough.title}
              onFocus={() => setFocused(index)} onClick={() => open(index)}>
              <span className="walkthrough-step-number" aria-hidden="true">{step ? index : ""}</span>
              <span className="walkthrough-step-icon" aria-hidden="true">{step ? stepIcon(step) : <BookOpenIcon aria-hidden="true" />}</span>
              <span className="walkthrough-row-text">
                <span className="walkthrough-row-title">{title}</span>
                {step && <span className="walkthrough-row-meta">{label}</span>}
              </span>
            </ListRow>
          )
        })}
      </div>
    </div>
  )
}
