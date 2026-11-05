// Native context menus (decision 0081): pops a validated menu over a window
// and resolves the chosen item id, or null when the menu closed without one.
import { Menu, type BrowserWindow, type MenuItemConstructorOptions } from "electron"
import type { ContextMenuRequest } from "./context-menu.ts"

export function popupContextMenu(window: BrowserWindow, request: ContextMenuRequest): Promise<string | null> {
  return new Promise((resolve) => {
    let chosen: string | null = null
    const template: MenuItemConstructorOptions[] = request.items.map((item) => item.kind === "separator"
      ? { type: "separator" }
      : {
        id: item.id,
        label: item.label,
        enabled: item.enabled,
        // A hint only: the page owns its keys, so nothing is registered.
        ...(item.accelerator === null ? {} : { accelerator: item.accelerator, registerAccelerator: false }),
        click: () => {
          chosen = item.id
        },
      })
    Menu.buildFromTemplate(template).popup({
      window,
      ...request.point,
      // Some platforms run click after the close callback: settle on the next turn.
      callback: () => setTimeout(() => resolve(chosen), 0),
    })
  })
}
