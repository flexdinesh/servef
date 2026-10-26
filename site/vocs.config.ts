import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'servef',
  description:
    'Browse any file in a local directory: rendered Markdown, image previews, formatted data, and highlighted source.',
  renderStrategy: 'full-static',
  accentColor: 'light-dark(#2563eb, #78a8ff)',
  sidebar: [
    { text: 'Introduction', link: '/' },
    { text: 'Getting started', link: '/getting-started' },
    { text: 'Usage', link: '/usage' },
    { text: 'Browsing', link: '/browsing' },
    { text: 'Code navigation', link: '/code-navigation' },
    { text: 'Walkthroughs', link: '/walkthroughs' },
  ],
  socials: [{ icon: 'github', link: 'https://github.com/flexdinesh/servef' }],
})
