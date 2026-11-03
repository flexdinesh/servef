// BoundedCache keeps the most recently used entries, up to max; reading an
// entry makes it the most recent.
export interface BoundedCache<V> {
  get(key: string): V | undefined
  has(key: string): boolean
  set(key: string, value: V): void
  delete(key: string): void
  clear(): void
  readonly size: number
}

export function boundedCache<V>(max: number): BoundedCache<V> {
  // entries iterate least recent first.
  const entries = new Map<string, V>()
  return {
    get(key) {
      const value = entries.get(key)
      if (value === undefined) return undefined
      entries.delete(key)
      entries.set(key, value)
      return value
    },
    has: (key) => entries.has(key),
    set(key, value) {
      entries.delete(key)
      entries.set(key, value)
      while (entries.size > max) {
        const oldest = entries.keys().next().value
        if (oldest === undefined) break
        entries.delete(oldest)
      }
    },
    delete: (key) => { entries.delete(key) },
    clear: () => entries.clear(),
    get size() { return entries.size },
  }
}
