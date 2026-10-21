// shiki-wasm-subset stands in for `shiki/wasm` when production builds bundle
// @pierre/diffs (vite.config.ts, decision 0024). Diffs use Shiki's JavaScript
// regex engine, so the Oniguruma WebAssembly build never ships or loads.
export default function wasm(): never {
  throw new Error("servef does not bundle Shiki's WebAssembly engine")
}
