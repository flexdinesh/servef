import { useCallback, useState } from "react"

import { readDiffOptions, storeDiffOptions, type DiffOptions } from "./diff-options.ts"

// useDiffOptions holds the diff toolbar's choices, remembered per browser.
export function useDiffOptions() {
  const [options, setOptions] = useState(readDiffOptions)
  const change = useCallback((update: Partial<DiffOptions>) => {
    setOptions((current) => {
      const next = { ...current, ...update }
      storeDiffOptions(next)
      return next
    })
  }, [])
  return { options, change }
}
