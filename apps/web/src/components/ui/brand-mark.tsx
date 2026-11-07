import { cn } from "cn"

const GRIDS = {
  small: {
    viewBox: "0 0 16 16",
    z: "M3 1H13V4L8 12H13V15H3V12L8 4H3z",
    rails: "M3 5H5V7.8L3 11zM13 11H11V8.2L13 5z",
  },
  display: {
    viewBox: "0 0 32 32",
    z: "M6 2H26V8.5L15 23.5H26V30H6V23.5L17 8.5H6z",
    rails: "M6 9.5H10V17.07L6 22.4zM26 22.5H22V14.93L26 9.6z",
  },
}

export type BrandMarkProps = {
  /** `small` is drawn on the 16 grid for 24px and under, `display` on the 32 grid. */
  size?: keyof typeof GRIDS
  /** `mono` takes the surrounding text color; `brand` draws the zero's rails in coral. */
  tone?: "mono" | "brand"
  className?: string
}

// BrandMark draws the Source Zero mark, a Z inside a zero (decision 0104). It is
// decorative: the surrounding link or heading names it.
export function BrandMark({ size = "small", tone = "mono", className }: BrandMarkProps) {
  const grid = GRIDS[size]
  return (
    <svg className={cn("ui-brand-mark", className)} data-tone={tone} viewBox={grid.viewBox} aria-hidden="true">
      <path fill="currentColor" d={grid.z} />
      <path className="ui-brand-mark-rails" fill="currentColor" d={grid.rails} />
    </svg>
  )
}
