import { EXPECTED_PROTOCOL } from "./daemon-contract.ts"
import type { EnsureFailure } from "./daemon.ts"
import { REMOTE_PATH_DIRS } from "./ssh-args.ts"
import type { SshFailure } from "./ssh-errors.ts"

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
          : [failure.detail, "Build servef with mise run dev:electron, or set SERVEF_DESKTOP_BINARY to a servef binary."],
        commands: packaged ? [] : ["mise run dev:electron"],
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

const RELEASES = "https://github.com/flexdinesh/servef/releases"
// INSTALL_SCRIPT installs or updates the latest release's CLI in ~/.local/bin
// or ~/bin (decision 0073); the scripts are the only supported install (0079).
const INSTALL_SCRIPT = "curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh"
const STABLE_RELEASE = /^\d+\.\d+\.\d+$/

// releaseOf is the servef release whose page and downloads the app links: its
// version when packaged as a stable release; null in dev (0.0.0-dev) and edge
// (0.0.0-edge) builds, which have no versioned release.
export function releaseOf(version: string, packaged: boolean): string | null {
  return packaged && STABLE_RELEASE.test(version) ? version : null
}
// DETAIL_LIMIT bounds the failure detail shown on a page (the stderr tail).
const DETAIL_LIMIT = 2048

// sshFailureInstructions explains an ssh connection failure (decision 0070).
// release is the app's servef release, or null for a dev build (links the
// latest release). Commands are shown text only; the app never runs them,
// and remote ones are labelled with the host they belong on.
export function sshFailureInstructions(failure: SshFailure, release: string | null): Instructions {
  const dest = failure.destination
  const detail = tail(failure.detail)
  const lead = detail === "" ? [] : [detail]
  const page = release === null ? `${RELEASES}/latest` : `${RELEASES}/tag/v${release}`
  const base = { commands: [], stopAndRetry: false }
  switch (failure.kind) {
    case "sshMissing":
      return {
        ...base,
        title: "OpenSSH is not installed",
        paragraphs: [...lead, "The app connects through the system ssh client. Install the OpenSSH client on this computer, then Retry."],
      }
    case "sshTooOld":
      return {
        ...base,
        title: "OpenSSH is too old",
        paragraphs: [...lead, "SSH connections need OpenSSH 6.7 or newer on this computer for Unix socket forwarding. Upgrade the OpenSSH client, then Retry."],
        commands: ["ssh -V"],
      }
    case "hostKey":
      return {
        ...base,
        title: `The host key of ${dest} is not verified`,
        paragraphs: [...lead, `Run ssh ${dest} in a terminal once to verify the host key, then Retry.`, "The app never accepts host keys for you."],
        commands: [`ssh ${dest}`],
      }
    case "auth":
      return {
        ...base,
        title: `SSH authentication to ${dest} failed`,
        paragraphs: [
          ...lead,
          "The app uses your SSH configuration in batch mode and never asks for passwords or passphrases.",
          `Load a key into your SSH agent with ssh-add, and make the second command succeed in a terminal, then Retry.`,
        ],
        commands: ["ssh-add", `ssh -o BatchMode=yes ${dest} true`],
      }
    case "unreachable":
      return {
        ...base,
        title: `Cannot reach ${dest}`,
        paragraphs: [...lead, "servef retries while a window for this host is open."],
      }
    case "servefMissing":
      return servefMissing(failure, lead, release, page)
    case "servefTooOld":
      return {
        ...base,
        title: `servef on ${dest} is too old`,
        paragraphs: [...lead, `This app needs a newer servef on ${dest}. Upgrade it there with the install script, or download a release from ${page}, then Retry.`],
        commands: [onHost(dest, INSTALL_SCRIPT)],
      }
    case "mismatch": {
      const status = failure.status
      const daemon = status === undefined
        ? `The servef daemon on ${dest} speaks a different protocol`
        : `The servef daemon on ${dest} (version ${status.version}) speaks protocol ${status.protocol}`
      const advice = status === undefined
        ? [`Use the same servef release on ${dest} and in this app (${page}).`]
        : status.protocol < EXPECTED_PROTOCOL
          ? [`servef on ${dest} is older than this app. Upgrade servef on ${dest}.`]
          : [`servef on ${dest} is newer than this app. Update the servef app (${page}).`]
      const upgrade = status !== undefined && status.protocol < EXPECTED_PROTOCOL ? [onHost(dest, INSTALL_SCRIPT)] : []
      return {
        title: `The servef daemon on ${dest} is incompatible`,
        paragraphs: [`${daemon}; this app needs protocol ${EXPECTED_PROTOCOL}.`, ...advice, "Then stop the running daemon and retry."],
        commands: [...upgrade, onHost(dest, "servef daemon stop")],
        stopAndRetry: true,
      }
    }
    case "forwardDenied":
      return {
        ...base,
        title: `${dest} refused the socket forward`,
        paragraphs: [
          ...lead,
          `The SSH server on ${dest} must allow Unix socket forwarding: AllowStreamLocalForwarding yes and no DisableForwarding in its sshd_config. It also needs OpenSSH 6.7 or newer.`,
          "Servers that listen on an address this computer can reach still open without the forward.",
        ],
      }
    case "runtimeDir":
      return {
        ...base,
        title: "The app's private runtime directory is not safe",
        paragraphs: [...lead, "The app keeps its SSH control sockets in a directory only you can use. Fix or remove the directory, then Retry."],
      }
    case "failed": {
      const socket = failure.status?.socket ?? ""
      return {
        ...base,
        title: `Connecting to ${dest} failed`,
        paragraphs: [
          detail === "" ? "The SSH connection failed." : detail,
          ...(socket === "" ? [] : [`See ${socket}.log on ${dest} for the daemon's log.`]),
        ],
      }
    }
  }
}

function servefMissing(failure: SshFailure, lead: string[], release: string | null, page: string): Instructions {
  const dest = failure.destination
  const always = [
    `servef must be on PATH for non-interactive SSH commands or in one of: ${REMOTE_PATH_DIRS.join(", ")}.`,
    `The app never installs anything on ${dest}. Install servef there, then Retry.`,
  ]
  const title = `servef is not installed on ${dest}`
  if (failure.os === "Darwin") {
    return {
      title,
      paragraphs: [...lead, `Install servef on ${dest} with the install script:`, ...always],
      commands: [onHost(dest, INSTALL_SCRIPT)],
      stopAndRetry: false,
    }
  }
  const arch = failure.os === "Linux" ? linuxArch(failure.arch) : null
  if (arch === null) {
    return {
      title,
      paragraphs: [...lead, `Releases cover macOS and Linux on amd64 and arm64. Install servef on ${dest} with the install script, or download a release from ${page}.`, ...always],
      commands: [onHost(dest, INSTALL_SCRIPT)],
      stopAndRetry: false,
    }
  }
  const archive = `servef_${release ?? "<version>"}_linux_${arch}`
  const download = release === null
    ? []
    : [onHost(dest, "mkdir -p ~/.local/bin", `curl -fsSL ${RELEASES}/download/v${release}/${archive}.tar.gz | tar -xz -C ~/.local/bin --strip-components=1 ${archive}/servef`)]
  return {
    title,
    paragraphs: [
      ...lead,
      `Install servef on ${dest} with the install script, or download ${archive}.tar.gz from ${page} and put its servef binary in ~/.local/bin.`,
      ...always,
    ],
    commands: [onHost(dest, INSTALL_SCRIPT), ...download],
    stopAndRetry: false,
  }
}

// linuxArch maps `uname -m` to a release archive architecture.
function linuxArch(machine: string | undefined): "amd64" | "arm64" | null {
  if (machine === "x86_64" || machine === "amd64") return "amd64"
  if (machine === "aarch64" || machine === "arm64") return "arm64"
  return null
}

// onHost labels a remote command block with the host it belongs on.
function onHost(dest: string, ...lines: string[]): string {
  return [`# on ${dest}`, ...lines].join("\n")
}

function tail(detail: string): string {
  const trimmed = detail.trim()
  return trimmed.length <= DETAIL_LIMIT ? trimmed : trimmed.slice(-DETAIL_LIMIT)
}
