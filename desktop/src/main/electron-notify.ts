// Notification adapter; SERVEF_DESKTOP_NOTICE_LOG turns notices into JSON lines.
import { Notification } from "electron"
import { appendFileSync } from "node:fs"
import type { Notice } from "./notifications.ts"

export interface NotifierOptions {
  // logFile, when set, receives one JSON line per notice instead of a notification.
  logFile: string | undefined
  onClick(notice: Notice): void
}

export interface Notifier {
  show(notice: Notice): void
}

export function createNotifier(options: NotifierOptions): Notifier {
  // Shown notifications stay referenced until dismissed so clicks still arrive.
  const live = new Set<Notification>()
  return {
    show(notice) {
      if (options.logFile !== undefined && options.logFile !== "") {
        const { kind, project, title, body, target } = notice
        try {
          appendFileSync(options.logFile, JSON.stringify({ kind, project, title, body, target }) + "\n")
        } catch (error) {
          console.error("servef: notice log:", error)
        }
        return
      }
      if (!Notification.isSupported()) return
      const notification = new Notification({ title: notice.title, body: notice.body })
      notification.on("click", () => {
        live.delete(notification)
        options.onClick(notice)
      })
      notification.on("close", () => live.delete(notification))
      live.add(notification)
      notification.show()
    },
  }
}
