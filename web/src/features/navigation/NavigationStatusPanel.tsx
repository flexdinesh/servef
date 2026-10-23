import { CheckIcon, CopyIcon } from "lucide-react"

import { Button } from "../../components/ui/button.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { useClipboardCopy } from "../../lib/clipboard.ts"
import type { LanguageServerInstall, NavigationStatus } from "../../api/navigation-contract.ts"
import { providerLabel, serverSummary } from "./navigation-status.ts"

function InstallCommand({ install }: { install: LanguageServerInstall }) {
  const copy = useClipboardCopy()
  return (
    <li className="navigation-install">
      <span className="navigation-install-label">{install.label}</span>
      {install.command ? <code>{install.command}</code> : <span>{install.docs && <a href={install.docs} target="_blank" rel="noreferrer">Installation instructions</a>}</span>}
      {install.command && <IconButton
        className="navigation-install-copy"
        size="md"
        label={`Copy ${install.label} install command`}
        title={copy.status === "failed" ? "Copy failed" : "Copy"}
        tone={copy.status === "copied" ? "success" : copy.status === "failed" ? "destructive" : "default"}
        data-status={copy.status}
        icon={copy.status === "copied" ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
        onClick={() => { void copy.copy(install.command) }}
      />}
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
      <div className="ui-popover status-process-panel status-navigation-panel">
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
          <Button variant="outline" onClick={onCheck}>Check again</Button>
          <Button variant="outline" onClick={onOpenSetting}>
            {enabled ? "Language server settings" : "Turn on in Settings"}
          </Button>
          {server?.docs && <a href={server.docs} target="_blank" rel="noreferrer">{server.name} docs</a>}
        </div>
      </div>
    </details>
  )
}
