import { useSidebarResize } from "./use-sidebar-resize.ts"

export function SidebarResizer() {
  const sidebar = useSidebarResize()
  return (
    <div
      className="sidebar-resizer"
      role="separator"
      aria-label="Resize file tree"
      aria-orientation="vertical"
      aria-valuemin={sidebar.minimumWidth}
      aria-valuemax={sidebar.maximumWidth}
      aria-valuenow={sidebar.width}
      tabIndex={0}
      onDoubleClick={sidebar.resetWidth}
      onKeyDown={sidebar.resizeWithKeyboard}
      onPointerDown={sidebar.startResize}
      onPointerMove={sidebar.moveResize}
      onPointerUp={sidebar.stopResize}
      onPointerCancel={sidebar.stopResize}
    />
  )
}
