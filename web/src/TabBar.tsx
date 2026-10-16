import { ContextMenu } from "@base-ui/react/context-menu"
import { XIcon } from "lucide-react"
import { useRef, type KeyboardEvent } from "react"

import { FileIcon } from "./FileIcon.tsx"
import "./tab-menu.css"

interface TabBarProps {
  activePath: string
  tabs: readonly string[]
  onClose(path: string): void
  onCloseMany(paths: readonly string[]): void
  onSelect(path: string): void
}

function fileName(path: string): string {
  return path.split("/").at(-1) ?? path
}

export function TabBar({ activePath, onClose, onCloseMany, onSelect, tabs }: TabBarProps) {
  const tabList = useRef<HTMLDivElement>(null)

  function focusTab(path: string) {
    const targets = tabList.current?.querySelectorAll<HTMLButtonElement>("[role=tab]")
    return Array.from(targets ?? []).find((target) => target.dataset.path === path)
      ?? tabList.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')
      ?? document.getElementById("document-pane")
  }

  function closeFromMenu(paths: readonly string[], path: string) {
    const index = tabs.indexOf(path)
    const remaining = tabs.filter((tab) => !paths.includes(tab))
    const focusPath = remaining.includes(path) ? path
      : remaining.includes(activePath) ? activePath
      : tabs.slice(index + 1).find((tab) => remaining.includes(tab))
        ?? tabs.slice(0, index).reverse().find((tab) => remaining.includes(tab)) ?? ""
    onCloseMany(paths)
    requestAnimationFrame(() => focusTab(focusPath)?.focus())
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) {
      event.preventDefault()
      const bounds = event.currentTarget.getBoundingClientRect()
      event.currentTarget.dispatchEvent(new MouseEvent("contextmenu", {
        bubbles: true, cancelable: true, clientX: bounds.left, clientY: bounds.bottom,
      }))
      return
    }
    if (event.key === "Delete" || event.key === "Backspace") {
      event.preventDefault()
      onClose(tabs[index] ?? "")
      return
    }
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
    event.preventDefault()
    const offset = event.key === "ArrowLeft" ? -1 : 1
    const nextIndex = (index + offset + tabs.length) % tabs.length
    const next = tabs[nextIndex]
    if (next) onSelect(next)
  }

  return (
    <div className="buffer-tabs" role="tablist" aria-label="Open documents" ref={tabList}>
      {tabs.map((path, index) => {
        const active = path === activePath
        return (
          <ContextMenu.Root key={path}>
            <ContextMenu.Trigger className={`buffer-tab${active ? " active" : ""}`}>
              <button
                className="buffer-tab-target"
                type="button"
                role="tab"
                data-path={path}
                aria-haspopup="menu"
                aria-controls="document-pane"
                aria-selected={active}
                title={path}
                onClick={() => onSelect(path)}
                onKeyDown={(event) => handleKeyDown(event, index)}
              >
                <FileIcon path={path} />
                <span>{fileName(path)}</span>
              </button>
              <button
                className="buffer-tab-close"
                type="button"
                aria-label={`Close ${path}`}
                title={`Close ${path}`}
                onClick={() => onClose(path)}
              >
                <XIcon aria-hidden="true" />
              </button>
            </ContextMenu.Trigger>
            <ContextMenu.Portal>
              <ContextMenu.Positioner className="tab-menu-positioner" sideOffset={2}>
                <ContextMenu.Popup className="tab-menu" aria-label={`Tab actions for ${fileName(path)}`}
                  finalFocus={() => focusTab(path)}>
                  <ContextMenu.Item className="tab-menu-item" onClick={() => closeFromMenu([path], path)}>
                    Close
                  </ContextMenu.Item>
                  <ContextMenu.Item className="tab-menu-item" disabled={tabs.length < 2}
                    onClick={() => closeFromMenu(tabs.filter((tab) => tab !== path), path)}>
                    Close Others
                  </ContextMenu.Item>
                  <ContextMenu.Item className="tab-menu-item" disabled={index === tabs.length - 1}
                    onClick={() => closeFromMenu(tabs.slice(index + 1), path)}>
                    Close to the Right
                  </ContextMenu.Item>
                  <ContextMenu.Separator className="tab-menu-separator" />
                  <ContextMenu.Item className="tab-menu-item" onClick={() => closeFromMenu(tabs, path)}>
                    Close All
                  </ContextMenu.Item>
                </ContextMenu.Popup>
              </ContextMenu.Positioner>
            </ContextMenu.Portal>
          </ContextMenu.Root>
        )
      })}
    </div>
  )
}
