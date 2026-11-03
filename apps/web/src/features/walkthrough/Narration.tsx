import { ChevronLeftIcon, ChevronRightIcon, XIcon } from "lucide-react"
import type { SyntheticEvent } from "react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import { Notice } from "../../components/ui/notice.tsx"
import type { WalkthroughStep } from "../../api/walkthrough-contract.ts"
import { DocumentPreview } from "../documents/DocumentPreview.tsx"
import type { Drift } from "./walkthrough-steps.ts"

const mac = typeof navigator !== "undefined" && navigator.platform.includes("Mac")
export const nextStepKeys = mac ? "⌘→" : "Ctrl+Right"
export const previousStepKeys = mac ? "⌘←" : "Ctrl+Left"

export interface NarrationProps {
  step: WalkthroughStep
  index: number
  count: number
  // drift is whether the step's code changed since it was written; missing
  // means its target is not in what the pane shows.
  drift: Drift | "missing"
  canMove(direction: 1 | -1): boolean
  onMove(direction: 1 | -1): void
  onExit(): void
  navigate(href: string): void
}

// Presses inside the zone stay out of the code around it, which selects
// lines and navigates names.
function stop(event: SyntheticEvent) {
  event.stopPropagation()
}

// Narration is a walkthrough step's story, inserted as a zone in the code it
// explains (decision 0047): below the step's lines in Source and diffs, at
// the top of a file or review without lines, and in the walkthrough's tab
// for notes.
export function Narration({ step, index, count, drift, canMove, onMove, onExit, navigate }: NarrationProps) {
  return (
    <section className="narration" aria-label={`Step ${index + 1} of ${count}: ${step.title}`}
      onPointerDown={stop} onMouseDown={stop} onClick={stop} onPointerOver={stop}>
      <header className="narration-header">
        <span className="narration-count">Step {index + 1} of {count}</span>
        <h2 className="narration-title">{step.title}</h2>
        <div className="narration-actions">
          <IconButton label="Previous Step" title={`Previous Step (${previousStepKeys})`}
            icon={<ChevronLeftIcon aria-hidden="true" />} disabledReason={canMove(-1) ? undefined : "This is the first step"}
            onClick={() => onMove(-1)} />
          <IconButton label="Next Step" title={`Next Step (${nextStepKeys})`}
            icon={<ChevronRightIcon aria-hidden="true" />} disabledReason={canMove(1) ? undefined : "This is the last step"}
            onClick={() => onMove(1)} />
          <IconButton label="Close Walkthrough" icon={<XIcon aria-hidden="true" />} onClick={onExit} />
        </div>
      </header>
      {drift === "changed" && (
        <Notice tone="warning" role="note" className="narration-notice">
          This code changed after the step was written, so it may not match the story.
        </Notice>
      )}
      {drift === "missing" && (
        <Notice tone="warning" role="note" className="narration-notice">
          This step's file isn't in this view anymore.
        </Notice>
      )}
      {step.bodyHTML && <div className="narration-body"><DocumentPreview html={step.bodyHTML} navigate={navigate} /></div>}
    </section>
  )
}
