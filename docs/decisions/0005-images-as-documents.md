# 0005: Images are browsable documents

- Status: Accepted
- Date: 2026-10-12

## Context

Images were only Markdown dependencies via `/api/image`; they never appeared
as documents.

## Decision

- `image` (png, jpg, jpeg, gif, webp, avif, ico, bmp) and `svg` families use
  preview kind `image`. Raster images have no text source; SVG keeps Source.
- `GET /api/raw?path=` serves indexed bytes only for image-preview families,
  with `image/*` content type, `X-Content-Type-Options: nosniff`, and
  `Content-Security-Policy: sandbox; default-src 'none'`. Others get 415.
- The viewer supports Fit (never upscale) and zoom (×1.25, 10–800%) over a
  checkerboard; no pan/background tools.
- `/api/image` stays the separate, root-confined Markdown dependency path.

## Consequences

- SVG renders through `<img>`, so embedded scripts never execute.
