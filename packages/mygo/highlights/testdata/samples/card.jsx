export default function Card({ title, children }) {
  return <section aria-label={title}><h2>{title}</h2>{children}<img src="a.png" /></section>
}
