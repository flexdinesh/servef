export const links = {
  docs: '/docs',
  getStarted: '/docs/getting-started',
  github: 'https://github.com/flexdinesh/servef',
  releases: 'https://github.com/flexdinesh/servef/releases',
  license: 'https://github.com/flexdinesh/servef/blob/main/LICENSE',
}

export type InstallMethod = { id: string; label: string; command: string }

export const installMethods: InstallMethod[] = [
  {
    id: 'script',
    label: 'Script',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh',
  },
  { id: 'brew', label: 'Homebrew', command: 'brew install flexdinesh/tap/servef' },
  { id: 'go', label: 'Go', command: 'go install github.com/flexdinesh/servef@latest' },
]
