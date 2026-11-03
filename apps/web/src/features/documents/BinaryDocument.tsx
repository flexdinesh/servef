import { formatBytes } from "../../lib/format-bytes.ts"

interface FileNoteProps {
  name: string
  size: number
}

function FileNote({ name, size, title, message }: FileNoteProps & { title: string; message: string }) {
  return (
    <div className="empty binary-document">
      <h1>{title}</h1>
      <p className="binary-document-meta">{name} · {formatBytes(size)}</p>
      <p>{message}</p>
    </div>
  )
}

export function BinaryDocument(props: FileNoteProps) {
  return <FileNote {...props} title="Binary file" message="servef shows text and images only." />
}

export function TooLargeDocument(props: FileNoteProps) {
  return <FileNote {...props} title="Too large to display" message="servef shows text files up to 32 MB." />
}
