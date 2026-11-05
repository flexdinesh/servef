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
    id: 'cli',
    label: 'CLI',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh',
  },
  {
    id: 'desktop',
    label: 'Desktop app',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop.sh | sh',
  },
]
