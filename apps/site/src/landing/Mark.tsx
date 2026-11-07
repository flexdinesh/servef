// The Source Zero mark, a Z inside a zero (decision 0104), on the 32 grid at the
// docs logo's 28px. Decorative: the wordmark beside it names the product.
export function Mark() {
  return (
    <svg className="landing-mark" viewBox="0 0 32 32" aria-hidden="true">
      <path className="landing-mark-z" d="M6 2H26V8.5L15 23.5H26V30H6V23.5L17 8.5H6z" />
      <path className="landing-mark-rails" d="M6 9.5H10V17.07L6 22.4zM26 22.5H22V14.93L26 9.6z" />
    </svg>
  )
}
