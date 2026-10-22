import { SourceDocument } from "./SourceDocument.tsx"

interface FormattedDocumentProps {
  text: string
  language: string | null
  label: string
}

// FormattedDocument shows the server's formatted text, highlighted as Source.
export function FormattedDocument({ text, language, label }: FormattedDocumentProps) {
  return <SourceDocument source={text} language={language} label={label} />
}
