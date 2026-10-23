import { useCallback, useEffect, useState } from "react"

import { isViewed, readViewed, withViewed, writeViewed, type ViewedMarks } from "./review-viewed.ts"

// useReviewViewed holds the project's Viewed marks, remembered in this
// browser; another browser tab's marks load when this one regains focus.
export function useReviewViewed(project: string) {
  const [marks, setMarks] = useState<ViewedMarks>(readViewed)
  useEffect(() => {
    const reload = () => setMarks(readViewed())
    window.addEventListener("focus", reload)
    return () => window.removeEventListener("focus", reload)
  }, [])
  const viewed = useCallback((path: string, blob: string) => isViewed(marks, project, path, blob), [marks, project])
  const mark = useCallback((path: string, blob: string, next: boolean) => {
    // Marks apply to the latest stored set, so concurrent tabs keep theirs.
    const updated = withViewed(readViewed(), project, path, blob, next)
    writeViewed(updated)
    setMarks(updated)
  }, [project])
  return { isViewed: viewed, mark }
}
