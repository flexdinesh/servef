# API fixtures

Committed API scenarios for frontend-only development and contract tests.

Run `pnpm dev:mock` to start Vite without the Go server. The default scenario
supports the index, a Markdown document, JSON with on-demand format success
(`config/settings.json`) and failure (`config/broken.json`), an image with raw
bytes (`assets/preview.png`), a binary note (`assets/sample.bin`), a large-file
warning (`logs/huge.log`), a too-large note (`logs/enormous.log`), search, features, metrics, and missing files.

Markdown under `testdata/markdown` remains the canonical server and browser-test
corpus. These JSON files cover transport and UI states only.
