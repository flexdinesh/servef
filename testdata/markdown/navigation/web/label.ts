// label describes a count for screen readers.
export function label(count: number): string {
  return count === 1 ? "1 item" : `${count} items`
}
