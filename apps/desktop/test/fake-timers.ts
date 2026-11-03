import type { TimerHandle, Timers } from "../src/main/timers.ts"

// FakeTimers drives Timers by hand: advance runs due callbacks in time order.
export interface FakeTimers {
  timers: Timers
  advance(ms: number): void
  // delays lists every setTimeout delay in call order.
  delays: number[]
  pending(): number
}

export function createFakeTimers(): FakeTimers {
  let now = 0
  let next = 0
  const pending = new Map<number, { at: number; callback: () => void }>()
  const delays: number[] = []
  const timers: Timers = {
    now: () => now,
    setTimeout(callback, ms): TimerHandle {
      next += 1
      pending.set(next, { at: now + ms, callback })
      delays.push(ms)
      return { id: next }
    },
    clearTimeout(handle) {
      pending.delete(handle.id)
    },
  }
  return {
    timers,
    delays,
    pending: () => pending.size,
    advance(ms) {
      const end = now + ms
      for (;;) {
        let due: [number, { at: number; callback: () => void }] | null = null
        for (const entry of pending) if (entry[1].at <= end && (due === null || entry[1].at < due[1].at)) due = entry
        if (due === null) break
        pending.delete(due[0])
        now = due[1].at
        due[1].callback()
      }
      now = end
    },
  }
}

// flush lets queued promise callbacks and I/O callbacks run.
export async function flush(rounds = 5): Promise<void> {
  for (let round = 0; round < rounds; round++) await new Promise((resolve) => setImmediate(resolve))
}
