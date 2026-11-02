'use client'

// Shares Vocs' theme storage (localStorage `vocs-theme`, `data-vocs-theme` on
// <html>) so the landing and /docs keep one theme.
const storageKey = 'vocs-theme'

function currentTheme(): 'light' | 'dark' {
  return document.documentElement.getAttribute('data-vocs-theme') === 'dark' ? 'dark' : 'light'
}

export function ThemeToggle() {
  function toggle() {
    const next = currentTheme() === 'dark' ? 'light' : 'dark'
    const html = document.documentElement
    html.setAttribute('data-vocs-theme', next)
    html.style.colorScheme = next
    try {
      localStorage.setItem(storageKey, next)
    } catch {
      // Storage may be unavailable; the theme still applies for this page.
    }
  }

  return (
    <button type="button" className="landing-theme-toggle" aria-label="Toggle color theme" onClick={toggle}>
      <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" className="landing-icon-sun">
        <circle cx="12" cy="12" r="4" fill="none" stroke="currentColor" strokeWidth="2" />
        <path
          d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
      <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" className="landing-icon-moon">
        <path
          d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinejoin="round"
        />
      </svg>
    </button>
  )
}
