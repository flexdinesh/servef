// Preload bridge v1 (decisions 0067, 0070): the only desktop API the UI sees,
// exposed on servef-app://local only. Remote and app windows get no bridge;
// the UI hides Open folder… and the menu button by feature detection.
import { contextBridge, ipcRenderer } from "electron"
import { APP_MENU_CHANNEL, BRIDGE_VERSION, isLocalFrame, OPEN_FOLDER_CHANNEL } from "../main/bridge.ts"

if (isLocalFrame(location.href)) {
  const openFolder = (): void => ipcRenderer.send(OPEN_FOLDER_CHANNEL)
  // macOS keeps its menu bar; elsewhere the title band's menu button pops it.
  contextBridge.exposeInMainWorld("servefDesktop", process.platform === "darwin"
    ? { version: BRIDGE_VERSION, openFolder }
    : {
      version: BRIDGE_VERSION,
      openFolder,
      showAppMenu: (x: number, y: number): void => ipcRenderer.send(APP_MENU_CHANNEL, { x, y }),
    })
}
