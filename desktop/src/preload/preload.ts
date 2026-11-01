// Preload bridge v1 (decisions 0067, 0070): the only desktop API the UI sees,
// exposed on servef-app://local only. Remote and app windows get no bridge;
// the UI hides Open folder… by feature detection.
import { contextBridge, ipcRenderer } from "electron"
import { BRIDGE_VERSION, isLocalFrame, OPEN_FOLDER_CHANNEL } from "../main/bridge.ts"

if (isLocalFrame(location.href)) {
  contextBridge.exposeInMainWorld("servefDesktop", {
    version: BRIDGE_VERSION,
    openFolder: (): void => ipcRenderer.send(OPEN_FOLDER_CHANNEL),
  })
}
