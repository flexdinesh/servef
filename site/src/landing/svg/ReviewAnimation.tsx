// A stacked diff: rows land, then each file in the rail gets a Viewed mark.
// Animation lives in landing.css (review-*).
export function ReviewAnimation() {
  const files = [
    { y: 74, name: 'server.go', stat: '+12 −4', check: 'review-check-1' },
    { y: 104, name: 'routes.go', stat: '+3 −1', check: 'review-check-2' },
    { y: 134, name: 'README.md', stat: '+8', check: 'review-check-3' },
  ]

  return (
    <svg className="landing-svg" viewBox="0 0 560 340" role="img" aria-labelledby="review-svg-title">
      <title id="review-svg-title">A stacked diff with a files rail where files are marked as viewed</title>
      <rect className="s-window" x="10" y="10" width="540" height="320" rx="12" />
      <path className="s-chrome" d="M22 10h516a12 12 0 0 1 12 12v22H10V22a12 12 0 0 1 12-12Z" />
      <text x="26" y="32" fontSize="11">
        main…feature/review · 3 files
      </text>

      {/* Files rail */}
      <line className="s-divider" x1="180" y1="44" x2="180" y2="330" />
      {files.map((file) => (
        <g key={file.name}>
          <text x="26" y={file.y + 14} fontSize="11">
            {file.name}
          </text>
          <text className="s-add-mark" x="112" y={file.y + 14} fontSize="9">
            {file.stat}
          </text>
          <g className={file.check}>
            <circle className="s-accent" cx="164" cy={file.y + 10} r="7" />
            <path
              d={`M160.5 ${file.y + 10}l2.5 2.5 4.5-5`}
              fill="none"
              stroke="var(--l-on-accent)"
              strokeWidth="1.6"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>
        </g>
      ))}

      {/* Diff */}
      <text className="s-strong-text" x="196" y="70" fontSize="11">
        internal/web/server.go
      </text>
      <text x="470" y="70" fontSize="10">
        Viewed
      </text>
      <rect className="review-check-1 s-accent" x="512" y="61" width="11" height="11" rx="3" />

      <rect className="s-line" x="232" y="90" width="200" height="7" rx="3.5" />
      <rect className="s-line" x="232" y="108" width="160" height="7" rx="3.5" />

      <g className="review-row review-row-1">
        <rect className="s-del" x="190" y="124" width="350" height="22" />
        <rect className="s-del-mark" x="198" y="133" width="8" height="2" />
        <rect className="s-del-mark" x="232" y="131" width="180" height="7" rx="3.5" opacity="0.6" />
      </g>
      <g className="review-row review-row-2">
        <rect className="s-add" x="190" y="146" width="350" height="22" />
        <rect className="s-add-mark" x="198" y="155" width="8" height="2" />
        <rect className="s-add-mark" x="201" y="152" width="2" height="8" />
        <rect className="s-add-mark" x="232" y="153" width="220" height="7" rx="3.5" opacity="0.6" />
      </g>
      <g className="review-row review-row-3">
        <rect className="s-add" x="190" y="168" width="350" height="22" />
        <rect className="s-add-mark" x="198" y="177" width="8" height="2" />
        <rect className="s-add-mark" x="201" y="174" width="2" height="8" />
        <rect className="s-add-mark" x="248" y="175" width="150" height="7" rx="3.5" opacity="0.6" />
      </g>
      <g className="review-row review-row-4">
        <rect className="s-add" x="190" y="190" width="350" height="22" />
        <rect className="s-add-mark" x="198" y="199" width="8" height="2" />
        <rect className="s-add-mark" x="201" y="196" width="2" height="8" />
        <rect className="s-add-mark" x="248" y="197" width="190" height="7" rx="3.5" opacity="0.6" />
      </g>

      <rect className="s-line" x="232" y="226" width="120" height="7" rx="3.5" />
      <rect className="s-line" x="232" y="244" width="240" height="7" rx="3.5" />
      <rect className="s-line" x="232" y="262" width="180" height="7" rx="3.5" />
      <rect className="s-chrome" x="190" y="282" width="350" height="24" rx="4" />
      <text x="320" y="298" fontSize="10">
        ··· 42 unchanged lines
      </text>
    </svg>
  )
}
