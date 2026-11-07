import { links } from './links'
import { Mark } from './Mark'
import { ThemeToggle } from './ThemeToggle'

export function Nav() {
  return (
    <header className="landing-nav">
      <a className="landing-wordmark" href="/">
        <Mark />
        <span className="landing-nav-name">Source Zero</span>
      </a>
      <nav aria-label="Primary" className="landing-nav-links">
        <a href="#features">Features</a>
        <a href={links.docs}>Docs</a>
        <a href={links.github}>GitHub</a>
        <ThemeToggle />
      </nav>
    </header>
  )
}
