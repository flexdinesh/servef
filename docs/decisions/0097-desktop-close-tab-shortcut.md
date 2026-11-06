# 0097: Desktop close shortcut closes the active tab

- Status: Accepted
- Date: 2026-11-05
- Revises: [0095](0095-desktop-app-is-mygo-electron-removed.md) (window menu), [0081](0081-desktop-bridge-v2-native-features.md) (optional bridge method)

Cmd+W on macOS and Ctrl+W on Linux close the active workspace tab. Closing the
final tab with this shortcut closes the app; the shortcut in an empty workspace
also closes the app. Tab close buttons and context menus keep their behavior.

The native Window menu registers Close Tab with CmdOrCtrl+W and dispatches the
cancelable `srcz:close-tab` event. React owns tab selection and cleanup, then
uses the optional, sender-checked `closeWindow()` bridge method after navigation
and workspace memory save the final close. The bridge stays version 2, with
feature detection as in 0081. Pages without a workspace retain native Close.
Browsers keep their own Cmd/Ctrl+W behavior. Quit and window controls stay native.
