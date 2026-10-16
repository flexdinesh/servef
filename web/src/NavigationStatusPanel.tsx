import { CheckIcon, CopyIcon } from "lucide-react"

import { useClipboardCopy } from "./clipboard.ts"
import type { LanguageServerInstall, NavigationStatus } from "./navigation-contract.ts"
import { providerLabel, serverSummary } from "./navigation-status.ts"

function InstallCommand({ install }: { install: LanguageServerInstall }) {
  const copy = useClipboardCopy()
  return (
    <li className="navigation-install">
      <span className="navigation-install-label">{install.label}</span>
      <code>{install.command}</code>
      <button
        type="button"
        className="navigation-install-copy"
        aria-label={`Copy ${install.label} install command`}
        title={copy.status === "failed" ? "Copy failed" : "Copy"}
        data-status={copy.status}
        onClick={() => { void copy.copy(install.command) }}
      >
        {copy.status === "copied" ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
      </button>
    </li>
  )
}

export interface NavigationStatusPanelProps {
  status: NavigationStatus
  onCheck(): void
  onOpenSetting(): void
}

// NavigationStatusPanel shows who answers code navigation for the open
// file and how to enable or install its language server.
export function NavigationStatusPanel({ status, onCheck, onOpenSetting }: NavigationStatusPanelProps) {
  const { server, languageServers } = status
  const usesServer = status.provider === "language-server" && server !== null
  return (
    <details className="status-process status-navigation" onKeyDown={(event) => {
      if (event.key === "Escape") event.currentTarget.open = false
    }}>
      <summary className="status-navigation-provider" title="Code navigation" data-provider={status.provider}
        data-state={server?.state}>
        {providerLabel(status)}
      </summary>
      <div className="status-process-panel status-navigation-panel">
        <strong>Code navigation</strong>
        <p>
          {usesServer
            ? `${server.language} navigation uses ${server.name}.`
            : "Built-in syntax navigation matches names across the project."}
        </p>
        {server && <p>{serverSummary(server, languageServers)}</p>}
        {server && !languageServers && server.state !== "missing" && (
          <p>Language servers are off. Turn them on for compiler-accurate results.</p>
        )}
        {server?.state === "missing" && (
          <>
            <p>Install {server.name} for compiler-accurate {server.language} results, then check again:</p>
            <ul className="navigation-installs">
              {server.install.map((install) => <InstallCommand key={install.label} install={install} />)}
            </ul>
          </>
        )}
        {server && languageServers && server.state !== "missing" && (
          <p>Servers start on first use and stop after {server.idleMinutes} idle minutes.</p>
        )}
        {server && (
          <div className="navigation-status-actions">
            {(server.state === "missing" || server.state === "failed") && (
              <button type="button" onClick={onCheck}>Check again</button>
            )}
            <button type="button" onClick={onOpenSetting}>
              {languageServers ? "Language server settings" : "Turn on in Settings"}
            </button>
            {server.docs && <a href={server.docs} target="_blank" rel="noreferrer">{server.name} docs</a>}
          </div>
        )}
      </div>
    </details>
  )
}
