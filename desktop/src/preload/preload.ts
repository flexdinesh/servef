// Preload bridge v1 (decision 0067): the only desktop API the UI sees.
import { contextBridge, ipcRenderer } from "electron"
import { BRIDGE_VERSION, OPEN_FOLDER_CHANNEL } from "../main/bridge.ts"

contextBridge.exposeInMainWorld("servefDesktop", {
  version: BRIDGE_VERSION,
  openFolder: (): void => ipcRenderer.send(OPEN_FOLDER_CHANNEL),
})
