// Preload bridge v2 (decisions 0067, 0070, 0081): the only desktop API the UI
// sees, exposed on servef-app://local only. Remote and app windows get no
// bridge; the UI hides Open folder…, the menu button, Reveal, and native
// menus by feature detection. Main validates every message.
import { contextBridge, ipcRenderer, webUtils } from "electron"
import {
  APP_MENU_CHANNEL, BRIDGE_VERSION, CONTEXT_MENU_CHANNEL, DROP_PATHS_CHANNEL, isLocalFrame, OPEN_FOLDER_CHANNEL, REVEAL_CHANNEL,
} from "../main/bridge.ts"

if (isLocalFrame(location.href)) {
  const bridge = {
    version: BRIDGE_VERSION,
    openFolder: (): void => ipcRenderer.send(OPEN_FOLDER_CHANNEL),
    // revealPath shows a project file in the OS file manager; false when refused.
    revealPath: (project: string, path: string): Promise<unknown> => ipcRenderer.invoke(REVEAL_CHANNEL, { project, path }),
    // showContextMenu pops a native menu and answers the chosen id, else null.
    showContextMenu: (items: unknown, x: number, y: number): Promise<unknown> => ipcRenderer.invoke(CONTEXT_MENU_CHANNEL, { items, x, y }),
  }
  // macOS keeps its menu bar; elsewhere the title band's menu button pops it.
  contextBridge.exposeInMainWorld("servefDesktop", process.platform === "darwin"
    ? bridge
    : { ...bridge, showAppMenu: (x: number, y: number): void => ipcRenderer.send(APP_MENU_CHANNEL, { x, y }) })

  // Files dragged from the OS: accept the drop unless the page handles it,
  // and send the dropped paths to main, which opens them as folders.
  const carriesFiles = (event: DragEvent): boolean => event.dataTransfer?.types.includes("Files") === true
  window.addEventListener("dragover", (event) => {
    if (event.defaultPrevented || !carriesFiles(event) || event.dataTransfer === null) return
    event.preventDefault()
    event.dataTransfer.dropEffect = "copy"
  })
  window.addEventListener("drop", (event) => {
    if (event.defaultPrevented || !carriesFiles(event) || event.dataTransfer === null) return
    event.preventDefault()
    const paths = Array.from(event.dataTransfer.files, (file) => webUtils.getPathForFile(file)).filter((path) => path !== "")
    if (paths.length > 0) ipcRenderer.send(DROP_PATHS_CHANNEL, paths)
  })
}
