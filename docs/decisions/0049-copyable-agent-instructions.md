# 0049: Copyable agent instructions for walkthroughs

- Status: Accepted
- Date: 2026-10-27
- Extends: [0047](0047-walkthroughs.md)

## Context

Publishing a walkthrough needs an agent that knows servef's URL, the
project's absolute path, the JSON format, and how to tell a good story. The
`servef-walkthrough` skill carries that guidance, but only for agents that
install it, and it finds servef by probing ports. The empty Walkthroughs view
named the endpoints and nothing else.

## Decision

- The Walkthroughs view header carries a Copy Agent Instructions icon on the
  list and on a walkthrough's steps; the empty list adds a labeled “Copy agent
  instructions” button. One click copies; the icon or label confirms or
  reports failure for `--duration-feedback`, announced politely.
- The copied text is a condensed prompt written in the frontend
  (`agent-instructions.ts`), not the skill: the page's origin, the project's
  absolute path, `POST /api/walkthroughs` and `GET /api/walkthrough-schema`
  (authoritative), the step and target model, the story guidance, a `curl`
  command, one JSON example, responses, and limits. It ends with
  “Walk me through: ” for the user's topic. Both screens copy the same text.
- A unit test checks every field of the example, and every required one,
  against `internal/walkthrough/schema.json`.

## Consequences

- The prompt works in any agent without the skill, and without probing ports.
- Its guidance duplicates part of the skill; the schema test catches field
  drift, not drift in the advice. Change both together.
- Under `mise run dev` the origin is Vite's, which proxies `/api`.
