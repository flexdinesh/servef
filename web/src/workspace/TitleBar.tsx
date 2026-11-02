import { MenuIcon } from "lucide-react"
import { useEffect, useState, type MouseEvent } from "react"

import { IconButton } from "../components/ui/icon-button.tsx"
import { desktopBridge } from "../lib/desktop-bridge.ts"

interface TitleBarProps {
  // title is the window title (document.title).
  title: string
}

// TitleBar is the desktop app's title band (decision 0075). It has no height
// in browsers; in desktop windows it fills the hidden native title bar,
// drags the window, and, outside macOS, opens the app menu.
export function TitleBar({ title }: TitleBarProps) {
  const showAppMenu = desktopBridge()?.showAppMenu ?? null
  const focused = useWindowFocused()

  const openMenu = (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    showAppMenu?.(rect.left, rect.bottom)
  }

  return (
    <div className="title-bar" data-window-focused={focused}>
      {showAppMenu && <IconButton className="title-bar-menu" label="Application menu" icon={<MenuIcon />} onClick={openMenu} />}
      <span className="title-bar-title" aria-hidden="true">{title}</span>
    </div>
  )
}

// useWindowFocused follows window focus, so the title dims in background
// windows like a native title bar.
function useWindowFocused(): boolean {
  const [focused, setFocused] = useState(() => document.hasFocus())
  useEffect(() => {
    const focus = () => setFocused(true)
    const blur = () => setFocused(false)
    window.addEventListener("focus", focus)
    window.addEventListener("blur", blur)
    return () => {
      window.removeEventListener("focus", focus)
      window.removeEventListener("blur", blur)
    }
  }, [])
  return focused
}
