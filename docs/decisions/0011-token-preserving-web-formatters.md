# 0011: Token-preserving formatters for HTML, CSS, and JavaScript

- Status: Accepted
- Date: 2026-10-15

## Context

JSON and YAML were the only formattable families. HTML, CSS, and JavaScript
are often minified or unevenly indented. `servef` must stay one Go binary,
and Format must preserve meaningful values (0003). Pure-Go options did not:
esbuild drops comments and rewrites literals (`'x'` → `"x"`, `0xFF` → `255`),
jsbeautifier-go corrupts modern syntax (`#p`, `??`, `?.`), and gohtml splits
inline elements, which changes rendered spacing.

## Decision

- `html`, `css`, and `javascript` (`.js`, `.mjs`, `.cjs`) use
  `FormattedCodeHandler` with small formatters built on the
  `tdewolff/parse` lexers. `jsx`, `typescript`, and `scss` stay source-only.
- Formatters write every token unchanged and decide only line breaks,
  indentation, and single spaces between tokens:
  - CSS: one declaration per line, nested blocks indented, a space after
    declaration colons only (selector colons such as `&:hover` are untouched).
  - JavaScript: breaks after `;` `{` `}` and before `}`, existing line
    breaks kept, never a break before `++`/`--`, so automatic semicolon
    insertion is unchanged. Regex versus division is decided from the previous
    token. `<` where an operand is expected means JSX: Format fails.
  - HTML: block elements get their own indented lines; inline runs, attribute
    values, comments, `pre`/`textarea`/`script`/`style`, and SVG/MathML stay
    verbatim. Only whitespace beside block elements and text line indentation
    change. The lexer lowercases tag and attribute names.
- Output is a reindent, not Prettier-style reflow: no line-length wrapping or
  operator spacing.

## Consequences

- Format never changes comments or literals; lexer errors (for example an
  unterminated JavaScript string) return 422 and keep Source.
- Embedded `<script>`/`<style>` content in HTML is not reformatted.
- Inline elements containing blocks are laid out as blocks; whitespace beside
  them can change.
