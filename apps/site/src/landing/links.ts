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
    id: 'desktop-cli',
    label: 'Desktop + CLI',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh',
  },
  {
    id: 'cli',
    label: 'CLI only',
    command: 'curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install.sh | sh -s -- --cli-only',
  },
]
