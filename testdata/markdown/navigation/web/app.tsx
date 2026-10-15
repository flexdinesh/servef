import { label } from "./label"

export function Badge({ count }: { count: number }) {
  return <span title={label(count)}>{count}</span>
}
