// System files the deb installs beside the app (decision 0072): file manager
// actions that open folders with `servef-desktop --open-path`. The AppImage
// has no system directories, so it gets none.
import path from "node:path"

// LINUX_INSTALLED_EXECUTABLE is where the deb installs the app (electron-builder
// installPrefix /opt, productName servef, executableName servef-desktop).
export const LINUX_INSTALLED_EXECUTABLE = "/opt/servef/servef-desktop"

// LINUX_EXTRA_FILES sources are relative to apps/desktop/.
export const LINUX_EXTRA_FILES: readonly { source: string; target: string }[] = [
  // Dolphin service menu (KF5 5.85+ and KF6 location).
  { source: "resources/linux/servef-desktop.servicemenu.desktop", target: "/usr/share/kio/servicemenus/servef-desktop.desktop" },
  // Nemo action.
  { source: "resources/linux/servef-desktop.nemo_action", target: "/usr/share/nemo/actions/servef-desktop.nemo_action" },
]

// debFpmArgs maps each extra file into the deb as an fpm `source=target` pair.
export function debFpmArgs(desktopDir: string): string[] {
  return LINUX_EXTRA_FILES.map((file) => `${path.resolve(desktopDir, file.source)}=${file.target}`)
}
