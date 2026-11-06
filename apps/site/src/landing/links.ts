export const links = {
  docs: '/docs',
  getStarted: '/docs/getting-started',
  github: 'https://github.com/flexdinesh/srcz',
  releases: 'https://github.com/flexdinesh/srcz/releases',
  license: 'https://github.com/flexdinesh/srcz/blob/main/LICENSE',
}

export type InstallMethod = { id: string; label: string; command: string }

export const installMethods: InstallMethod[] = [
  {
    id: 'cli',
    label: 'CLI',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh',
  },
  {
    id: 'desktop',
    label: 'Desktop app',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install-desktop.sh | sh',
  },
]
