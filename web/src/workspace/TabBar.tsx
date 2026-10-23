import { ContextMenu } from "@base-ui/react/context-menu"
import { GitCompareArrowsIcon, XIcon } from "lucide-react"
import { useEffect, useRef, type KeyboardEvent } from "react"

import { FileIcon } from "../components/FileIcon.tsx"
import { Button } from "../components/ui/button.tsx"
import { IconButton } from "../components/ui/icon-button.tsx"
import { menuItemClass, menuPopupClass, menuPositionerClass, menuSeparatorClass } from "../components/ui/menu.ts"
import { includesTab, sameTab, tabKey, type TabName, type WorkspaceTab } from "./workspace-tabs.ts"

interface TabBarProps {
  active: WorkspaceTab | null
  tabs: readonly WorkspaceTab[]
  // name labels a tab; review tabs say what they compare.
  name(tab: WorkspaceTab): TabName
  onClose(tab: WorkspaceTab): void
  onCloseMany(tabs: readonly WorkspaceTab[]): void
  onSelect(tab: WorkspaceTab): void
}

export function TabBar({ active, name: tabName, onClose, onCloseMany, onSelect, tabs }: TabBarProps) {
  const tabList = useRef<HTMLDivElement>(null)
  const activeKey = active ? tabKey(active) : ""

  // Keep the active tab visible in an overflowing strip, as editors do. Tabs
  // widen once the UI font loads, so reveal again when the active one resizes.
  useEffect(() => {
    if (!activeKey) return
    const target = Array.from(tabList.current?.querySelectorAll<HTMLElement>("[role=tab]") ?? [])
      .find((tab) => tab.dataset.tab === activeKey)
    if (!target) return
    const reveal = () => target.scrollIntoView({ block: "nearest", inline: "nearest" })
    reveal()
    const observer = new ResizeObserver(reveal)
    observer.observe(target)
    return () => observer.disconnect()
  }, [activeKey, tabs.length])

  function focusTab(tab: WorkspaceTab | null) {
    const targets = tabList.current?.querySelectorAll<HTMLButtonElement>("[role=tab]")
    return Array.from(targets ?? []).find((target) => tab !== null && target.dataset.tab === tabKey(tab))
      ?? tabList.current?.querySelector<HTMLButtonElement>('[aria-selected="true"]')
      ?? document.getElementById("document-pane")
  }

  function closeFromMenu(closing: readonly WorkspaceTab[], tab: WorkspaceTab) {
    const index = tabs.findIndex((entry) => sameTab(entry, tab))
    const remaining = tabs.filter((entry) => !includesTab(closing, entry))
    const focus = includesTab(remaining, tab) ? tab
      : active && includesTab(remaining, active) ? active
      : tabs.slice(index + 1).find((entry) => includesTab(remaining, entry))
        ?? tabs.slice(0, index).reverse().find((entry) => includesTab(remaining, entry)) ?? null
    onCloseMany(closing)
    requestAnimationFrame(() => focusTab(focus)?.focus())
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
      const tab = tabs[index]
      if (tab) onClose(tab)
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
      {tabs.map((tab, index) => {
        const selected = sameTab(tab, active)
        const { label: name, title } = tabName(tab)
        return (
          <ContextMenu.Root key={tabKey(tab)}>
            <ContextMenu.Trigger className={`buffer-tab${selected ? " active" : ""}`}>
              <Button
                className="buffer-tab-target"
                variant="ghost"
                size="inline"
                role="tab"
                data-tab={tabKey(tab)}
                aria-haspopup="menu"
                aria-controls="document-pane"
                aria-selected={selected}
                title={title}
                onClick={() => onSelect(tab)}
                onKeyDown={(event) => handleKeyDown(event, index)}
              >
                {tab.kind === "review" ? <GitCompareArrowsIcon className="file-icon" aria-hidden="true" /> : <FileIcon path={tab.path} />}
                <span>{name}</span>
              </Button>
              <IconButton
                className="buffer-tab-close"
                size="xs"
                label={`Close ${title}`}
                icon={<XIcon aria-hidden="true" />}
                onClick={() => onClose(tab)}
              />
            </ContextMenu.Trigger>
            <ContextMenu.Portal>
              <ContextMenu.Positioner className={menuPositionerClass} sideOffset={2}>
                <ContextMenu.Popup className={menuPopupClass} aria-label={`Tab actions for ${name}`}
                  finalFocus={() => focusTab(tab)}>
                  <ContextMenu.Item className={menuItemClass} onClick={() => closeFromMenu([tab], tab)}>
                    Close
                  </ContextMenu.Item>
                  <ContextMenu.Item className={menuItemClass} disabled={tabs.length < 2}
                    onClick={() => closeFromMenu(tabs.filter((entry) => !sameTab(entry, tab)), tab)}>
                    Close Others
                  </ContextMenu.Item>
                  <ContextMenu.Item className={menuItemClass} disabled={index === tabs.length - 1}
                    onClick={() => closeFromMenu(tabs.slice(index + 1), tab)}>
                    Close to the Right
                  </ContextMenu.Item>
                  <ContextMenu.Separator className={menuSeparatorClass} />
                  <ContextMenu.Item className={menuItemClass} onClick={() => closeFromMenu(tabs, tab)}>
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
