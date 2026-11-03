import { isRecord } from "../../../web/src/api/http.ts"
import { parseMenuPoint, type MenuPoint } from "./titlebar.ts"

// Native context menus (decision 0081): a local page sends item descriptors
// and a point; main pops an OS menu there and answers the chosen id. Pages
// are untrusted input: every field is checked and any invalid item refuses
// the whole menu.

export const MAX_MENU_ITEMS = 32
export const MAX_ITEM_ID = 64
export const MAX_ITEM_LABEL = 128
export const MAX_ACCELERATOR = 40

export type ContextMenuItem =
  | { kind: "item"; id: string; label: string; enabled: boolean; accelerator: string | null }
  | { kind: "separator" }

export interface ContextMenuRequest {
  items: ContextMenuItem[]
  point: MenuPoint
}

const ITEM_ID = /^[A-Za-z0-9._:-]+$/
// CONTROL matches C0 controls and DEL, which no label needs.
const CONTROL = /[\u0000-\u001f\u007f]/
const MODIFIER = "(?:CommandOrControl|CmdOrCtrl|Command|Cmd|Control|Ctrl|Alt|Option|AltGr|Shift|Super|Meta)"
const KEY = "(?:[A-Z0-9]|F(?:[1-9]|1[0-9]|2[0-4])|Plus|Space|Tab|Backspace|Delete|Insert|Return|Enter|Up|Down|Left|Right|Home|End|PageUp|PageDown|Escape|Esc|[,./;'`=\\[\\]\\\\-])"
const ACCELERATOR = new RegExp(`^(?:${MODIFIER}\\+)*${KEY}$`)

// parseContextMenuRequest validates {items, x, y} from a renderer against the
// sender's content size. Items are {id, label, enabled?, accelerator?} or
// {separator: true}; ids are unique; accelerators are display hints in
// Electron's accelerator grammar. Leading, trailing, and repeated separators
// are dropped; a menu needs at least one item. Anything else is null.
export function parseContextMenuRequest(value: unknown, size: { width: number; height: number }): ContextMenuRequest | null {
  if (!isRecord(value)) return null
  const point = parseMenuPoint({ x: value.x, y: value.y }, size)
  const raw = value.items
  if (point === null || !Array.isArray(raw) || raw.length === 0 || raw.length > MAX_MENU_ITEMS) return null
  const items: ContextMenuItem[] = []
  const ids = new Set<string>()
  for (const entry of raw) {
    const item = parseItem(entry)
    if (item === null) return null
    if (item.kind === "separator") {
      if (items.length > 0 && items.at(-1)?.kind !== "separator") items.push(item)
      continue
    }
    if (ids.has(item.id)) return null
    ids.add(item.id)
    items.push(item)
  }
  if (items.at(-1)?.kind === "separator") items.pop()
  return items.length === 0 ? null : { items, point }
}

function parseItem(value: unknown): ContextMenuItem | null {
  if (!isRecord(value)) return null
  if (value.separator === true) return Object.keys(value).length === 1 ? { kind: "separator" } : null
  const { id, label, enabled, accelerator } = value
  if (typeof id !== "string" || id.length > MAX_ITEM_ID || !ITEM_ID.test(id)) return null
  if (typeof label !== "string" || label.trim() === "" || label.length > MAX_ITEM_LABEL || CONTROL.test(label)) return null
  if (enabled !== undefined && typeof enabled !== "boolean") return null
  if (accelerator !== undefined && (typeof accelerator !== "string" || accelerator.length > MAX_ACCELERATOR || !ACCELERATOR.test(accelerator))) return null
  return { kind: "item", id, label, enabled: enabled ?? true, accelerator: accelerator ?? null }
}
