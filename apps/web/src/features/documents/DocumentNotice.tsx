import { Notice } from "../../components/ui/notice.tsx"
import { formatFailureMessage } from "./document-format.ts"
import { formatBytes } from "../../lib/format-bytes.ts"
import { shortHash } from "../../lib/commit-hash.ts"

interface LargeFileNoticeProps {
  size: number
  searchExcluded: boolean
}

export function LargeFileNotice({ size, searchExcluded }: LargeFileNoticeProps) {
  return (
    <Notice tone="warning" layout="bar" className="document-notice" role="note">
      Large file ({formatBytes(size)}): preview and formatting are off.
      {searchExcluded && " Not included in search."}
    </Notice>
  )
}

// SnapshotNotice marks a document read from a commit, not the working tree.
export function SnapshotNotice({ at }: { at: string }) {
  return (
    <Notice layout="bar" className="document-notice" role="note">
      Snapshot at <code title={at}>{shortHash(at)}</code>: read-only, without preview or code navigation.
    </Notice>
  )
}

export function FormatErrorNotice({ reason }: { reason: string }) {
  return (
    <Notice tone="error" layout="bar" className="document-notice">
      {formatFailureMessage(reason)}
    </Notice>
  )
}
