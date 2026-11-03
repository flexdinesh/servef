import { createFileTreeIconResolver, getBuiltInSpriteSheet } from "@pierre/trees"
import { FileTextIcon } from "lucide-react"
import { useEffect } from "react"

// Matches the explorer's Pierre icon configuration so tabs and tree rows agree.
const iconSet = "complete"
const resolver = createFileTreeIconResolver({ set: iconSet, colored: true })

let spriteMounted = false

// Pierre keeps its sprite inside the tree's shadow root; tabs need one light-DOM copy.
// Append it last so document anchors earlier in the page win any id collisions.
function mountSprite() {
  if (spriteMounted) return
  spriteMounted = true
  const host = document.createElement("div")
  host.className = "file-icon-sprite"
  host.setAttribute("aria-hidden", "true")
  host.innerHTML = getBuiltInSpriteSheet(iconSet)
  document.body.append(host)
}

export function FileIcon({ path }: { path: string }) {
  useEffect(mountSprite, [])
  const icon = resolver.resolveIcon("file-tree-icon-file", path)
  if (!icon.token) return <FileTextIcon className="file-icon" aria-hidden="true" />
  return <svg className="file-icon" data-icon-token={icon.token} viewBox={icon.viewBox ?? "0 0 16 16"} aria-hidden="true">
    <use href={`#${icon.name}`} />
  </svg>
}
