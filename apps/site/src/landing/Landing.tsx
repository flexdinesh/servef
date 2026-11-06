import './landing.css'
import { Footer } from './Footer'
import { FeatureSection } from './FeatureSection'
import { InstallTabs } from './InstallTabs'
import { links } from './links'
import { Nav } from './Nav'
import { DesktopAnimation } from './svg/DesktopAnimation'
import { HeroAnimation } from './svg/HeroAnimation'
import { ReviewAnimation } from './svg/ReviewAnimation'
import { WalkthroughAnimation } from './svg/WalkthroughAnimation'

export function Landing() {
  return (
    <div className="landing">
      <a className="landing-skip" href="#landing-main">
        Skip to content
      </a>
      <Nav />
      <div id="landing-main">
        <section className="landing-hero">
          <div className="landing-hero-copy">
            <p className="landing-eyebrow">Local files, one binary</p>
            <h1>
              Every file in a folder.
              <br />
              <span className="landing-muted">One command away.</span>
            </h1>
            <p className="landing-lede">
              Source Zero serves any local directory as a fast, read-only workspace: rendered
              Markdown, image previews, formatted data, highlighted source, and diffs.
            </p>
            <InstallTabs />
            <div className="landing-ctas">
              <a className="landing-button landing-button-primary" href={links.getStarted}>
                Get started
              </a>
              <a className="landing-button" href={links.docs}>
                Read the docs
              </a>
            </div>
          </div>
          <div className="landing-hero-visual">
            <HeroAnimation />
          </div>
        </section>

        <div id="features" className="landing-features">
          <FeatureSection
            eyebrow="Source control"
            title="Review changes like a pull request."
            body="Uncommitted work, commits, branches against any base, and commit ranges in one stacked diff, with line counts and Viewed marks."
            visual={<ReviewAnimation />}
          />
          <FeatureSection
            eyebrow="Walkthroughs"
            title="Let agents walk you through the code."
            body="Agents publish guided tours of files and diffs, with narration inline beside the lines that matter."
            visual={<WalkthroughAnimation />}
            reverse
          />
          <FeatureSection
            eyebrow="Desktop"
            title="Your project, in its own window."
            body="The desktop app opens folders and srcz:// links in the OS web view, with native menus and a daemon every terminal shares."
            visual={<DesktopAnimation />}
          />
        </div>

        <section className="landing-cta">
          <h2>One command. Then get back to reading.</h2>
          <InstallTabs />
          <div className="landing-ctas landing-ctas-center">
            <a className="landing-button landing-button-primary" href={links.getStarted}>
              Install now
            </a>
            <a className="landing-button" href={links.docs}>
              Read the docs
            </a>
          </div>
        </section>
      </div>
      <Footer />
    </div>
  )
}
