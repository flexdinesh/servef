import { useEffect, useState } from "react"

import { desktopBridge, type NotificationStatus } from "../../lib/desktop-bridge.ts"
import type { Settings } from "../../settings/settings.ts"

import { notificationConfig, sameNotificationStatus } from "./notifications.ts"

// useNotificationSync tells the desktop app what to notify about: on load, when
// the project or settings change, and on focus, which also refreshes whether
// notices can reach the user and retries a connection that stopped being
// trusted. Without a desktop app it does nothing and returns null.
export function useNotificationSync(project: string, settings: Settings): NotificationStatus | null {
  const [status, setStatus] = useState<NotificationStatus | null>(null)
  const { commits, branches, edits, walkthroughs } = notificationConfig(project, settings)

  useEffect(() => {
    const configure = desktopBridge()?.configureNotifications
    if (!configure) return
    let current = true
    const sync = () => {
      void configure({ project, commits, branches, edits, walkthroughs }).then((next) => {
        if (current) setStatus((previous) => sameNotificationStatus(previous, next) ? previous : next)
      })
    }
    sync()
    window.addEventListener("focus", sync)
    return () => {
      current = false
      window.removeEventListener("focus", sync)
    }
  }, [project, commits, branches, edits, walkthroughs])

  return status
}
