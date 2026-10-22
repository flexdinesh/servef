import { CheckIcon, CopyIcon } from "lucide-react"

import { useClipboardCopy } from "../../lib/clipboard.ts"
import type { LanguageServerInstall, NavigationStatus } from "../../api/navigation-contract.ts"
import { providerLabel, serverSummary } from "./navigation-status.ts"

function InstallCommand({ install }: { install: LanguageServerInstall }) {
  const copy = useClipboardCopy()
  return (
    <li className="navigation-install">
      <span className="navigation-install-label">{install.label}</span>
      {install.command ? <code>{install.command}</code> : <span>{install.docs && <a href={install.docs} target="_blank" rel="noreferrer">Installation instructions</a>}</span>}
      {install.command && <button
        type="button"
        className="navigation-install-copy"
        aria-label={`Copy ${install.label} install command`}
        title={copy.status === "failed" ? "Copy failed" : "Copy"}
        data-status={copy.status}
        onClick={() => { void copy.copy(install.command) }}
      >
        {copy.status === "copied" ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
      </button>}
      {install.prerequisites && install.prerequisites.length > 0 && <p className="navigation-install-prerequisites">Requires: {install.prerequisites.join(", ")}.</p>}
      {install.command && install.docs && <a className="navigation-install-prerequisites" href={install.docs} target="_blank" rel="noreferrer">Installation instructions</a>}
      <span className="visually-hidden" role="status">{copy.status === "copied" ? "Install command copied" : copy.status === "failed" ? "Copy failed" : ""}</span>
    </li>
  )
}

export interface NavigationStatusPanelProps {
  status: NavigationStatus | null
  onCheck(): void
  onOpenSetting(): void
}

// NavigationStatusPanel shows who answers code navigation for the open
// file and how to enable or install its language server.
export function NavigationStatusPanel({ status, onCheck, onOpenSetting }: NavigationStatusPanelProps) {
  const server = status?.server
  const enabled = status?.enabled ?? true
  return (
    <details className="status-process status-navigation" onKeyDown={(event) => {
      if (event.key === "Escape") event.currentTarget.open = false
    }}>
      <summary className="status-navigation-provider" title="Language server status" data-state={server?.state}>
        {providerLabel(status)}
      </summary>
      <div className="status-process-panel status-navigation-panel">
        <strong>Language server</strong>
        {!status && <p>Could not check language server availability. Check again to retry.</p>}
        {status && !status.supported && <p>No language server is configured for this language.</p>}
        {status?.supported && !status.eligible && status.reason && <p>{status.reason}</p>}
        {status && !enabled && <p>Language servers are off. Turn them on for code navigation.</p>}
        {server && <p>{serverSummary(server, enabled)}</p>}
        {server?.state === "missing" && (
          <>
            <p>Install {server.name} on the machine running servef, then check again:</p>
            <ul className="navigation-installs">
              {server.install.map((install) => <InstallCommand key={install.label} install={install} />)}
            </ul>
          </>
        )}
        {server && enabled && server.state !== "missing" && (
          <p>Servers start on first use and stop after {server.idleMinutes} idle minutes.</p>
        )}
        {server?.capabilities && (
          <p>Go to Definition: {server.capabilities.definition ? "supported" : "unavailable"}. Find References: {server.capabilities.references ? "supported" : "unavailable"}.</p>
        )}
        <div className="navigation-status-actions">
          <button type="button" onClick={onCheck}>Check again</button>
          <button type="button" onClick={onOpenSetting}>
            {enabled ? "Language server settings" : "Turn on in Settings"}
          </button>
          {server?.docs && <a href={server.docs} target="_blank" rel="noreferrer">{server.name} docs</a>}
        </div>
      </div>
    </details>
  )
}
