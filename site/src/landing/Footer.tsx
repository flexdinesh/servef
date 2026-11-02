import { links } from './links'

export function Footer() {
  return (
    <footer className="landing-footer">
      <div className="landing-footer-brand">
        <span className="landing-wordmark">servef</span>
        <span className="landing-muted">MIT License</span>
      </div>
      <nav aria-label="Footer" className="landing-footer-links">
        <a href={links.docs}>Docs</a>
        <a href={links.github}>GitHub</a>
        <a href={links.releases}>Releases</a>
        <a href={links.license}>License</a>
      </nav>
    </footer>
  )
}
