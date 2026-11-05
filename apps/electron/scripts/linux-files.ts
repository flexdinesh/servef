// System files the deb installs beside the app (decision 0072): file manager
// actions that open folders with `servef-electron --open-path`. The AppImage
// has no system directories, so it gets none.
import path from "node:path"

// LINUX_INSTALLED_EXECUTABLE is where the deb installs the app (electron-builder
// installPrefix /opt, productName servef, executableName servef-electron).
export const LINUX_INSTALLED_EXECUTABLE = "/opt/servef/servef-electron"

// LINUX_EXTRA_FILES sources are relative to apps/electron/.
export const LINUX_EXTRA_FILES: readonly { source: string; target: string }[] = [
  // Dolphin service menu (KF5 5.85+ and KF6 location).
  { source: "resources/linux/servef-electron.servicemenu.desktop", target: "/usr/share/kio/servicemenus/servef-electron.desktop" },
  // Nemo action.
  { source: "resources/linux/servef-electron.nemo_action", target: "/usr/share/nemo/actions/servef-electron.nemo_action" },
]

// DEB_RENAMED_FROM is this package's name before the rename (decision 0092).
// Installing this deb makes apt remove it; both own paths under /opt/servef.
export const DEB_RENAMED_FROM = "servef-desktop"

// debFpmArgs declares the old package's relations, then maps each extra file
// into the deb as an fpm `source=target` pair.
export function debFpmArgs(desktopDir: string): string[] {
  return [
    "--replaces", DEB_RENAMED_FROM,
    "--conflicts", DEB_RENAMED_FROM,
    "--provides", DEB_RENAMED_FROM,
    ...LINUX_EXTRA_FILES.map((file) => `${path.resolve(desktopDir, file.source)}=${file.target}`),
  ]
}
