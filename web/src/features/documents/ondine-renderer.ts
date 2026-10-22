import { create } from "ondine/compiler/wasm"
import allModuleUrl from "ondine/compiler/wasm/all.wasm?url"

export type OndineRenderer = create.ReturnType

let pending: Promise<OndineRenderer> | null = null

// loadOndineRenderer compiles the all-diagram module once per page and shares
// the renderer. A failed load clears the cache so a later attempt fetches again.
export function loadOndineRenderer(): Promise<OndineRenderer> {
  pending ??= WebAssembly.compileStreaming(fetch(allModuleUrl)).then(
    (all) => create({ modules: { all } }),
    (error: unknown) => {
      pending = null
      throw error
    },
  )
  return pending
}
