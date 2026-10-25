import { useEffect, useState, type RefObject } from "react"

// useNarrowElement reports whether element is narrower than width rem, as it
// resizes. Rem follows the root font size, so the Font Size setting applies.
export function useNarrowElement(element: RefObject<HTMLElement | null>, width: number): boolean {
  const [narrow, setNarrow] = useState(false)
  useEffect(() => {
    const target = element.current
    if (!target) return
    const measure = () => {
      const rem = Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
      setNarrow(target.clientWidth > 0 && target.clientWidth < width * rem)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(target)
    return () => observer.disconnect()
  }, [element, width])
  return narrow
}
