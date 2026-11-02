import type { ReactNode } from 'react'

type FeatureSectionProps = {
  eyebrow: string
  title: string
  body: string
  visual: ReactNode
  reverse?: boolean
}

export function FeatureSection({ eyebrow, title, body, visual, reverse = false }: FeatureSectionProps) {
  return (
    <section className={reverse ? 'landing-feature landing-feature-reverse' : 'landing-feature'}>
      <div className="landing-feature-copy">
        <p className="landing-eyebrow">{eyebrow}</p>
        <h2>{title}</h2>
        <p className="landing-lede">{body}</p>
      </div>
      <div className="landing-feature-visual">{visual}</div>
    </section>
  )
}
