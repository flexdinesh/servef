interface MermaidSourceProps {
  error?: string
  source: string
}

export function MermaidSource({ error, source }: MermaidSourceProps) {
  return (
    <>
      <pre><code className="language-mermaid">{source}</code></pre>
      {error && <p className="mermaid-error" role="status">{error}</p>}
    </>
  )
}
