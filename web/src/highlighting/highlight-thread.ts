// MAIN_THREAD_MAX_CHARACTERS is the largest text Highlights tokenizes on the
// main thread; larger text goes to a worker so the UI never blocks
// (decision 0029). Diffs count both sides, as Pierre's pool does.
export const MAIN_THREAD_MAX_CHARACTERS = 100_000

// highlightsOnMainThread reports whether text of this length highlights
// without a worker round trip.
export function highlightsOnMainThread(characters: number): boolean {
  return characters <= MAIN_THREAD_MAX_CHARACTERS
}
