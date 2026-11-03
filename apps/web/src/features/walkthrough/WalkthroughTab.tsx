import { useMemo } from "react"

import { Button } from "../../components/ui/button.tsx"
import { StatusMessage } from "../../components/ui/notice.tsx"
import { DocumentPreview } from "../documents/DocumentPreview.tsx"
import { commitAge, commitDate, type CompareContext } from "../source-control/compare.ts"
import { Narration } from "./Narration.tsx"
import type { WalkthroughDetail } from "./use-walkthroughs.ts"
import { stepLabel, stepTarget, type StepPosition } from "./walkthrough-steps.ts"

interface WalkthroughTabProps {
  detail: WalkthroughDetail
  // position is the route's step, null for the cover.
  position: StepPosition
  context: CompareContext
  canMove(direction: 1 | -1): boolean
  onMove(direction: 1 | -1): void
  onGoTo(position: StepPosition): void
  onExit(): void
  onRetry(): void
  navigate(href: string): void
}

// WalkthroughTab is a walkthrough's own tab (decision 0047): its cover (the
// title, summary, and outline) and, on a note step, that note's narration
// above it.
export function WalkthroughTab({ canMove, context, detail, navigate, position, onExit, onGoTo, onMove, onRetry }: WalkthroughTabProps) {
  const updatedAt = detail.kind === "ready" ? detail.walkthrough.updatedAt : ""
  const now = useMemo(() => updatedAt ? Date.now() : 0, [updatedAt])
  if (detail.kind === "missing") {
    return (
      <div className="empty">
        <h1>Walkthrough unavailable</h1>
        <p>It was deleted, or another project published it.</p>
      </div>
    )
  }
  if (detail.kind === "error") {
    return (
      <div className="error" role="alert">
        <p>{detail.message}</p>
        <Button variant="outline" onClick={onRetry}>Try again</Button>
      </div>
    )
  }
  if (detail.kind !== "ready") return <StatusMessage>Loading walkthrough…</StatusMessage>
  const { walkthrough } = detail
  const note = position === null ? undefined : walkthrough.steps[position]
  const count = walkthrough.steps.length
  return (
    <div className="walkthrough-tab">
      {note && position !== null && (
        <Narration step={note} index={position} count={count} drift="unknown"
          canMove={canMove} onMove={onMove} onExit={onExit} navigate={navigate} />
      )}
      <header className="walkthrough-cover">
        <p className="walkthrough-cover-meta" title={`Updated ${commitDate(walkthrough.updatedAt)}`}>
          Walkthrough · {count.toLocaleString()} {count === 1 ? "step" : "steps"} · updated {commitAge(walkthrough.updatedAt, now).long}
        </p>
        <h1 className="walkthrough-cover-title">{walkthrough.title}</h1>
      </header>
      {walkthrough.summaryHTML && <DocumentPreview html={walkthrough.summaryHTML} navigate={navigate} />}
      {position === null && count > 0 && (
        <Button className="walkthrough-start" onClick={() => onGoTo(0)}>Start walkthrough</Button>
      )}
      <nav aria-label="Steps" className="walkthrough-outline">
        <ol>
          {walkthrough.steps.map((step, index) => {
            const target = stepTarget(step)
            return (
              <li key={index} data-current={index === position || undefined}>
                <Button variant="link" size="inline" aria-current={index === position ? "step" : undefined}
                  onClick={() => onGoTo(index)}>{step.title}</Button>
                {target.kind !== "note" && <span className="walkthrough-outline-label">{stepLabel(step, context)}</span>}
              </li>
            )
          })}
        </ol>
      </nav>
    </div>
  )
}
