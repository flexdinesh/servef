import { formatFailureMessage } from "./document-format.ts"
import { formatBytes } from "../../lib/format-bytes.ts"

interface LargeFileNoticeProps {
  size: number
  searchExcluded: boolean
}

export function LargeFileNotice({ size, searchExcluded }: LargeFileNoticeProps) {
  return (
    <p className="document-notice document-notice-warning" role="note">
      Large file ({formatBytes(size)}): preview and formatting are off.
      {searchExcluded && " Not included in search."}
    </p>
  )
}

export function FormatErrorNotice({ reason }: { reason: string }) {
  return (
    <p className="document-notice document-notice-error" role="alert">
      {formatFailureMessage(reason)}
    </p>
  )
}
