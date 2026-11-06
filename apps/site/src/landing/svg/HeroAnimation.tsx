// Terminal runs `srcz .`, then the workspace opens with a file tree and a
// rendered Markdown preview. Animation lives in landing.css (hero-*).
export function HeroAnimation() {
  return (
    <svg className="landing-svg" viewBox="0 0 600 440" role="img" aria-labelledby="hero-svg-title">
      <title id="hero-svg-title">Running srcz in a terminal opens a browser workspace for the folder</title>

      {/* Browser workspace */}
      <g className="hero-browser">
        <rect className="s-window" x="60" y="70" width="530" height="360" />
        <rect className="s-chrome" x="60" y="70" width="530" height="36" />
        <rect className="s-dot" x="76" y="84" width="8" height="8" />
        <rect className="s-dot" x="90" y="84" width="8" height="8" />
        <rect className="s-dot" x="104" y="84" width="8" height="8" />
        <rect className="s-window" x="210" y="79" width="230" height="18" />
        <text x="222" y="92" fontSize="10">
          127.0.0.1:7980
        </text>

        {/* Sidebar file tree */}
        <line className="s-divider" x1="200" y1="106" x2="200" y2="430" />
        <rect className="hero-select s-accent-subtle" x="66" y="124" width="128" height="22" />
        <text x="78" y="139" fontSize="11">
          README.md
        </text>
        <text x="78" y="165" fontSize="11">
          diagram.svg
        </text>
        <text x="78" y="191" fontSize="11">
          config.yaml
        </text>
        <text x="78" y="217" fontSize="11">
          src/
        </text>
        <text x="92" y="243" fontSize="11">
          main.go
        </text>
        <text x="92" y="269" fontSize="11">
          server.go
        </text>

        {/* Markdown preview */}
        <g className="hero-preview">
          <rect className="s-line-strong" x="224" y="128" width="190" height="14" />
          <rect className="s-line" x="224" y="156" width="330" height="7" />
          <rect className="s-line" x="224" y="171" width="300" height="7" />
          <rect className="s-accent-subtle" x="224" y="194" width="330" height="96" />
          <g>
            <rect className="s-chrome" x="224" y="306" width="330" height="96" />
            <rect className="s-accent" x="240" y="322" width="60" height="7" />
            <rect className="s-line-strong" x="308" y="322" width="90" height="7" />
            <rect className="s-line" x="256" y="340" width="140" height="7" />
            <rect className="s-add-mark" x="256" y="358" width="70" height="7" />
            <rect className="s-line" x="240" y="376" width="40" height="7" />
          </g>
          <g>
            <rect className="s-accent" x="282" y="224" width="36" height="36" opacity="0.5" />
            <path className="s-accent" d="M350 270l40-46 36 46Z" opacity="0.7" />
            <path className="s-accent" d="M400 270l30-30 30 30Z" opacity="0.4" />
          </g>
        </g>
      </g>

      {/* Terminal */}
      <g>
        <rect className="s-window" x="10" y="10" width="300" height="110" />
        <rect className="s-chrome" x="10" y="10" width="300" height="22" />
        <rect className="s-dot" x="22" y="17" width="7" height="7" />
        <rect className="s-dot" x="34" y="17" width="7" height="7" />
        <rect className="s-dot" x="46" y="17" width="7" height="7" />
        <text className="s-accent-text" x="26" y="62" fontSize="14">
          $
        </text>
        <text className="hero-type s-strong-text" x="42" y="62" fontSize="14">
          srcz .
        </text>
        <text className="hero-url" x="26" y="92" fontSize="12">
          → serving http://127.0.0.1:7980
        </text>
      </g>
    </svg>
  )
}
