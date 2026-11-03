import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import type { Walkthrough } from "../../api/walkthrough-contract.ts"
import {
  readWalkthroughMemory, rememberSeen, rememberStep, unseenCount, writeWalkthroughMemory, type WalkthroughMemory,
} from "./walkthrough-memory.ts"
import { coverRoute, parseStep, stepAfter, stepRoute, type StepPosition, type StepRoute } from "./walkthrough-steps.ts"
import { useWalkthrough, useWalkthroughs, type WalkthroughDetail } from "./use-walkthroughs.ts"

interface SessionOptions {
  project: string
  // routeId and routeStep are the route's wt and step parameters.
  routeId: string
  routeStep: number | undefined
  // viewShown is set while the Walkthroughs view is visible.
  viewShown: boolean
  // go opens a route; focusDocument moves focus to the document.
  go(route: StepRoute, focusDocument: boolean): void
  // leave opens the route's own target without the walkthrough.
  leave(): void
}

function isTextField(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || target.closest("input, textarea, select, [contenteditable=true]") !== null)
}

// useWalkthroughSession owns which walkthrough the user follows and where
// (decision 0047). The route's wt and step place the narration; leaving the
// route keeps the walkthrough selected, so Next resumes from the last step.
// Ctrl/Cmd+Right and Ctrl/Cmd+Left step while one is selected.
export function useWalkthroughSession({ project, routeId, routeStep, viewShown, go, leave }: SessionOptions) {
  const list = useWalkthroughs(project)
  const [memory, setMemory] = useState<WalkthroughMemory>(readWalkthroughMemory)
  const update = useCallback((change: (current: WalkthroughMemory) => WalkthroughMemory) => {
    setMemory((current) => {
      const next = change(readWalkthroughMemory())
      if (next !== current) writeWalkthroughMemory(next)
      return next
    })
  }, [])
  const [chosen, setChosen] = useState("")
  // The route's walkthrough becomes the selected one.
  const [followed, setFollowed] = useState(routeId)
  if (routeId !== followed) {
    setFollowed(routeId)
    if (routeId) setChosen(routeId)
  }
  const selectedId = routeId || chosen
  const summary = list.result?.find((entry) => entry.id === selectedId)
  const { detail, retry } = useWalkthrough(project, selectedId, summary?.updatedAt ?? "")
  const walkthrough = detail.kind === "ready" ? detail.walkthrough : null
  const count = walkthrough?.steps.length ?? 0
  const routed = routeId !== "" && routeId === selectedId
  const remembered = memory.steps[project]?.[selectedId]
  const position: StepPosition = routed
    ? parseStep(routeStep, count)
    : remembered !== undefined && remembered < count ? remembered : null

  // visited marks the steps opened this session, per walkthrough.
  const [visited, setVisited] = useState<ReadonlyMap<string, ReadonlySet<number>>>(new Map())
  useEffect(() => {
    if (!routed || position === null) return
    update((current) => rememberStep(current, project, selectedId, position))
    setVisited((current) => {
      const steps = current.get(selectedId)
      if (steps?.has(position)) return current
      const next = new Map(current)
      next.set(selectedId, new Set([...(steps ?? []), position]))
      return next
    })
  }, [position, project, routed, selectedId, update])

  // The view marks every listed walkthrough seen while it shows.
  const newest = list.result?.[0]?.updatedAt
  useEffect(() => {
    if (viewShown && newest) update((current) => rememberSeen(current, project, newest))
  }, [newest, project, update, viewShown])
  const unseen = useMemo(() => unseenCount((list.result ?? []).map((entry) => entry.updatedAt), memory.seen[project]), [list.result, memory.seen, project])

  const goTo = useCallback((target: StepPosition, focusDocument = true) => {
    if (!walkthrough) return
    const step = target === null ? undefined : walkthrough.steps[target]
    go(target === null || !step ? coverRoute(project, walkthrough.id) : stepRoute(project, walkthrough.id, target, step), focusDocument)
  }, [go, project, walkthrough])
  const move = useCallback((direction: 1 | -1) => {
    const next = stepAfter(position, count, direction)
    if (next !== undefined) goTo(next)
  }, [count, goTo, position])

  // open shows a walkthrough's cover, or resumes it where the user left off.
  const open = useCallback((id: string) => {
    setChosen(id)
    go(coverRoute(project, id), false)
  }, [go, project])
  // close returns the view to the list; the route keeps its target.
  const close = useCallback(() => {
    setChosen("")
    if (routeId) leave()
  }, [leave, routeId])

  const keys = useRef({ move, active: walkthrough !== null })
  useEffect(() => { keys.current = { move, active: walkthrough !== null } })
  useEffect(() => {
    const step = (event: KeyboardEvent) => {
      if (!keys.current.active || event.defaultPrevented || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return
      if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return
      if (isTextField(event.target)) return
      event.preventDefault()
      keys.current.move(event.key === "ArrowRight" ? 1 : -1)
    }
    document.addEventListener("keydown", step)
    return () => document.removeEventListener("keydown", step)
  }, [])

  return {
    list,
    unseen,
    selectedId,
    detail: selectedId ? detail : { kind: "none" } satisfies WalkthroughDetail,
    retry,
    walkthrough,
    // routed is set while the route follows the selected walkthrough.
    routed,
    position,
    visited: visited.get(selectedId) ?? new Set<number>(),
    canMove: (direction: 1 | -1) => stepAfter(position, count, direction) !== undefined,
    goTo,
    move,
    open,
    close,
  }
}

export type WalkthroughSession = ReturnType<typeof useWalkthroughSession>
export type { Walkthrough }
