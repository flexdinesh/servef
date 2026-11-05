import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'servef',
  description:
    'Browse any file in a local directory: rendered Markdown, image previews, formatted data, and highlighted source.',
  renderStrategy: 'full-static',
  accentColor: 'light-dark(#2563eb, #78a8ff)',
  sidebar: [
    { text: 'Introduction', link: '/docs' },
    { text: 'Getting started', link: '/docs/getting-started' },
    { text: 'Usage', link: '/docs/usage' },
    { text: 'Desktop app', link: '/docs/desktop' },
    { text: 'mygo app (experimental)', link: '/docs/mygo' },
    { text: 'Remote hosts', link: '/docs/remote' },
    { text: 'Browsing', link: '/docs/browsing' },
    { text: 'Code navigation', link: '/docs/code-navigation' },
    { text: 'Walkthroughs', link: '/docs/walkthroughs' },
  ],
  socials: [{ icon: 'github', link: 'https://github.com/flexdinesh/servef' }],
})
