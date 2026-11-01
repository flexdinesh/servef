import { EXPECTED_PROTOCOL } from "./daemon-contract.ts"
import type { EnsureFailure } from "./daemon.ts"

export interface Instructions {
  title: string
  paragraphs: string[]
  commands: string[]
  // stopAndRetry offers "Stop the running daemon and retry" (mismatch only).
  stopAndRetry: boolean
}

export function failureInstructions(failure: EnsureFailure, packaged: boolean): Instructions {
  switch (failure.kind) {
    case "missing":
      return {
        title: "servef is not available",
        paragraphs: packaged
          ? [failure.detail, "The app's bundled servef binary is missing. Reinstall the servef app."]
          : [failure.detail, "Build servef with mise run dev:desktop, or set SERVEF_DESKTOP_BINARY to a servef binary."],
        commands: packaged ? [] : ["mise run dev:desktop"],
        stopAndRetry: false,
      }
    case "mismatch": {
      const status = failure.status
      const daemon = status === undefined
        ? "The running servef daemon speaks a different protocol"
        : `The running servef daemon (version ${status.version}) speaks protocol ${status.protocol}`
      return {
        title: "The servef daemon is incompatible",
        paragraphs: [`${daemon}; this app needs protocol ${EXPECTED_PROTOCOL}.`, "Stop the running daemon, then retry. The app starts a compatible one."],
        commands: ["servef daemon stop"],
        stopAndRetry: true,
      }
    }
    case "failed": {
      const socket = failure.status?.socket ?? ""
      return {
        title: "The servef daemon did not start",
        paragraphs: [
          failure.detail === "" ? "servef daemon ensure failed." : failure.detail,
          socket === "" ? "Run servef daemon ensure in a terminal for details." : `See ${socket}.log for the daemon's log.`,
        ],
        commands: [],
        stopAndRetry: false,
      }
    }
  }
}
