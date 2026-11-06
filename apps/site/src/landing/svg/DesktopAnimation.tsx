// A srcz:// link from a terminal opens a project in the desktop app's window.
// Animation lives in landing.css (desk-*).
export function DesktopAnimation() {
  return (
    <svg className="landing-svg" viewBox="0 0 560 340" role="img" aria-labelledby="desk-svg-title">
      <title id="desk-svg-title">A srcz link from a terminal opens a project in the desktop app's window</title>

      {/* Link from a terminal */}
      <g className="desk-link">
        <rect className="s-window" x="10" y="14" width="300" height="34" />
        <rect className="s-accent" x="24" y="25" width="12" height="12" />
        <text x="44" y="35" fontSize="11">
          srcz://local/?project=/srv/api
        </text>
      </g>

      {/* Project window */}
      <g className="desk-window">
        <rect className="s-window" x="40" y="64" width="440" height="266" />
        <rect className="s-chrome" x="40" y="64" width="440" height="34" />
        <rect className="s-dot" x="54" y="77" width="8" height="8" />
        <rect className="s-dot" x="68" y="77" width="8" height="8" />
        <rect className="s-dot" x="82" y="77" width="8" height="8" />
        <text x="190" y="85" fontSize="11">
          api · Source Zero
        </text>
        <line className="s-divider" x1="150" y1="98" x2="150" y2="330" />
        <rect className="s-line" x="56" y="116" width="70" height="7" />
        <rect className="s-accent-subtle" x="48" y="132" width="96" height="20" />
        <rect className="s-accent" x="56" y="139" width="60" height="7" />
        <rect className="s-line" x="56" y="164" width="80" height="7" />
        <rect className="s-line" x="56" y="184" width="50" height="7" />
        <rect className="s-line" x="56" y="204" width="74" height="7" />
        <rect className="s-line-strong" x="170" y="118" width="160" height="12" />
        <rect className="s-line" x="170" y="146" width="280" height="7" />
        <rect className="s-line" x="170" y="162" width="250" height="7" />
        <rect className="s-line" x="170" y="178" width="270" height="7" />
        <rect className="s-chrome" x="170" y="200" width="290" height="100" />
        <rect className="s-line" x="186" y="216" width="120" height="7" />
        <rect className="s-line" x="202" y="232" width="180" height="7" />
        <rect className="s-line" x="202" y="248" width="140" height="7" />
        <rect className="s-line" x="186" y="264" width="60" height="7" />
      </g>
    </svg>
  )
}
