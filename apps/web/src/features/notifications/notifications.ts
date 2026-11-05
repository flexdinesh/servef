import type { NotificationConfig, NotificationStatus } from "../../lib/desktop-bridge.ts"
import type { Settings } from "../../settings/settings.ts"

// notificationConfig is what the desktop app needs to follow a connection's
// projects (decision 0099).
export function notificationConfig(project: string, settings: Settings): NotificationConfig {
  return {
    project,
    commits: settings.notifyCommits === "on",
    branches: settings.notifyBranches === "on",
    edits: settings.notifyEdits === "on",
    walkthroughs: settings.notifyWalkthroughs === "on",
  }
}

// notificationProblem says why notices cannot reach the user, or null when
// they can. The app learns of a denial only when it first tries to show one.
export function notificationProblem(status: NotificationStatus | null): string | null {
  if (!status) return null
  if (!status.supported) return "This computer has no notification service, so these settings have no effect."
  if (status.denied) return "macOS is blocking notifications from servef. Turn them on in System Settings › Notifications."
  return null
}

export function sameNotificationStatus(a: NotificationStatus | null, b: NotificationStatus | null): boolean {
  return a?.supported === b?.supported && a?.denied === b?.denied
}
