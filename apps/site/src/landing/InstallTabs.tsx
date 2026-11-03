'use client'

import { useState } from 'react'
import { installMethods } from './links'

export function InstallTabs() {
  const [selected, setSelected] = useState(0)
  const [copied, setCopied] = useState(false)
  const method = installMethods[selected] ?? installMethods[0]
  if (method === undefined) return null

  async function copy(command: string) {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="landing-install">
      <div className="landing-install-tabs" role="tablist" aria-label="Install method">
        {installMethods.map((m, index) => (
          <button
            key={m.id}
            type="button"
            role="tab"
            aria-selected={index === selected}
            className="landing-install-tab"
            onClick={() => {
              setSelected(index)
              setCopied(false)
            }}
          >
            {m.label}
          </button>
        ))}
      </div>
      <div className="landing-install-command" role="tabpanel">
        <code>
          <span className="landing-install-prompt" aria-hidden="true">
            $
          </span>
          {method.command}
        </code>
        <button type="button" className="landing-install-copy" onClick={() => void copy(method.command)}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
    </div>
  )
}
