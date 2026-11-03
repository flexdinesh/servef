// Timers is the clock seam pure modules use so tests can drive time.
export interface Timers {
  now(): number
  setTimeout(callback: () => void, ms: number): TimerHandle
  clearTimeout(handle: TimerHandle): void
}

export interface TimerHandle {
  readonly id: number
}

// systemTimers wraps the global clock; handles map to numeric ids.
export const systemTimers: Timers = createSystemTimers()

function createSystemTimers(): Timers {
  const pending = new Map<number, ReturnType<typeof setTimeout>>()
  let next = 0
  return {
    now: () => Date.now(),
    setTimeout(callback, ms) {
      next += 1
      const id = next
      pending.set(id, setTimeout(() => {
        pending.delete(id)
        callback()
      }, ms))
      return { id }
    },
    clearTimeout(handle) {
      const timer = pending.get(handle.id)
      if (timer === undefined) return
      clearTimeout(timer)
      pending.delete(handle.id)
    },
  }
}
