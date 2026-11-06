// A walkthrough steps a highlight through code while narration changes beside
// it. Animation lives in landing.css (walk-*).
export function WalkthroughAnimation() {
  const notes = [
    { className: 'walk-note-1', title: '1 · Entry point', lines: [150, 120] },
    { className: 'walk-note-2', title: '2 · Routing', lines: [140, 110] },
    { className: 'walk-note-3', title: '3 · The fix', lines: [150, 90] },
  ]
  const code = [70, 160, 120, 200, 90, 180, 140, 110, 170, 60, 150, 130]

  return (
    <svg className="landing-svg" viewBox="0 0 560 340" role="img" aria-labelledby="walk-svg-title">
      <title id="walk-svg-title">A walkthrough highlights code step by step with narration beside it</title>
      <rect className="s-window" x="10" y="10" width="540" height="320" />
      <rect className="s-chrome" x="10" y="10" width="540" height="34" />
      <text x="26" y="32" fontSize="11">
        Walkthrough · How requests reach a handler
      </text>

      {/* Step progress */}
      <rect className="s-line" x="474" y="24" width="56" height="4" />
      <rect className="walk-step s-accent" x="474" y="24" width="24" height="4" />

      {/* Code with moving highlight */}
      <rect className="walk-highlight s-accent-subtle" x="10" y="62" width="320" height="44" />
      <rect className="walk-highlight s-accent" x="10" y="62" width="3" height="44" />
      {code.map((width, index) => (
        <g key={index}>
          <text x="24" y={76 + index * 21} fontSize="9">
            {String(index + 1).padStart(2, ' ')}
          </text>
          <rect
            className={index % 3 === 0 ? 's-line-strong' : 's-line'}
            x={index % 4 === 0 ? 50 : 66}
            y={70 + index * 21}
            width={width}
            height="7"
          />
        </g>
      ))}

      {/* Narration */}
      <line className="s-divider" x1="340" y1="44" x2="340" y2="330" />
      {notes.map((note) => (
        <g key={note.className} className={note.className}>
          <rect className="s-chrome" x="356" y="62" width="178" height="110" />
          <text className="s-accent-text" x="372" y="86" fontSize="11">
            {note.title}
          </text>
          <rect className="s-line-strong" x="372" y="100" width={note.lines[0]} height="7" />
          <rect className="s-line" x="372" y="116" width={note.lines[1]} height="7" />
          <rect className="s-line" x="372" y="132" width="130" height="7" />
          <text x="372" y="160" fontSize="9">
            Next →
          </text>
        </g>
      ))}
    </svg>
  )
}
