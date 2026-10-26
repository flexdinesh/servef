import { ChevronLeftIcon, ChevronRightIcon, ListOrderedIcon, XIcon } from "lucide-react"

import { IconButton } from "../../components/ui/icon-button.tsx"
import type { Walkthrough } from "../../api/walkthrough-contract.ts"
import { nextStepKeys, previousStepKeys } from "./Narration.tsx"
import type { StepPosition } from "./walkthrough-steps.ts"

interface WalkthroughStepBarProps {
  walkthrough: Walkthrough
  position: StepPosition
  presenting: boolean
  canMove(direction: 1 | -1): boolean
  onMove(direction: 1 | -1): void
  // onSteps opens the step list; phones show it in the drawer.
  onSteps?: () => void
  onStopPresenting(): void
}

// WalkthroughStepBar floats over the document while presenting, and on
// phones while following a walkthrough (decision 0047): Previous, where the
// step is, Next, and the step list or the way out of presenting.
export function WalkthroughStepBar({ canMove, onMove, onSteps, onStopPresenting, position, presenting, walkthrough }: WalkthroughStepBarProps) {
  const count = walkthrough.steps.length
  const step = position === null ? undefined : walkthrough.steps[position]
  return (
    <div className="walkthrough-step-bar" role="toolbar" aria-label={`Walkthrough: ${walkthrough.title}`}>
      <IconButton size="md" label="Previous Step" title={`Previous Step (${previousStepKeys})`}
        icon={<ChevronLeftIcon aria-hidden="true" />} disabledReason={canMove(-1) ? undefined : "This is the overview"}
        onClick={() => onMove(-1)} />
      <span className="walkthrough-step-bar-position" aria-live="polite">
        {position === null ? "Overview" : `${(position + 1).toLocaleString()} / ${count.toLocaleString()}`}
      </span>
      <span className="walkthrough-step-bar-title">{step?.title ?? walkthrough.title}</span>
      <IconButton size="md" label="Next Step" title={`Next Step (${nextStepKeys})`}
        icon={<ChevronRightIcon aria-hidden="true" />} disabledReason={canMove(1) ? undefined : "This is the last step"}
        onClick={() => onMove(1)} />
      {onSteps && <IconButton size="md" label="Steps" icon={<ListOrderedIcon aria-hidden="true" />} onClick={onSteps} />}
      {presenting && (
        <IconButton size="md" label="Stop Presenting" title="Stop Presenting (Esc)" icon={<XIcon aria-hidden="true" />}
          onClick={onStopPresenting} />
      )}
    </div>
  )
}
