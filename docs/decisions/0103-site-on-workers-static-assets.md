# 0103: Site on Workers Static Assets

- Status: Accepted
- Date: 2026-11-07
- Supersedes: [0074](0074-landing-page-and-cloudflare-pages.md) hosting on
  Cloudflare Pages and no `wrangler.toml`

## Context

0074 planned Cloudflare Pages Git integration with its settings in the
dashboard, and the site was never deployed. Cloudflare's dashboard now starts
new applications as Workers and tucks the Pages flow behind a side link. We
chose the Workers flow for the first deploy instead of the older one.

## Decision

- The site deploys as a Worker named `sourcezero-docs`, with static assets only
  and no Worker code. It connects to the repository through Workers Builds.
- `apps/site/wrangler.jsonc` owns the Worker name, compatibility date, assets
  directory (`./dist/public`), and 404 handling. The file stays with the site,
  so 0074's objection to a root config no longer applies; the deploy command
  names it with `--config`.
- `not_found_handling` is `404-page`. The default, `none`, would not serve the
  site's `404.html`.
- The build still runs from the repository root, with the filtered pnpm install
  and `pnpm build:site`. Build and deploy commands stay in the dashboard
  (`docs/release.md`).
- Wrangler stays out of the lockfile. The deploy command runs it through `npx`.

## Consequences

- The Worker name in the dashboard and the config must match.
- `npx` resolves Wrangler at deploy time, so deploys do not pin its version or
  pass through `minimumReleaseAge`.
- The Cloudflare build image's Node 26 and pnpm 11 support stays unverified
  until the first deploy. If it fails, build in GitHub Actions and run
  `wrangler deploy` there.
