import { CopyButton } from "../../components/ui/copy-button.tsx"

interface CodeBlockControlsProps {
  language: string | null
  source: string
}

export function CodeBlockControls({ language, source }: CodeBlockControlsProps) {
  const description = language ? `${language} code` : "code"

  return (
    <div className="code-block-controls">
      <CopyButton
        text={source}
        label={`Copy ${description}`}
        variant={language ? "chip" : "raised"}
        className="code-block-control"
        iconClassName="code-block-control-copy"
      >
        {language && <span className="code-block-control-language">{language}</span>}
      </CopyButton>
    </div>
  )
}
