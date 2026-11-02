// Pure window titles (decision 0070).

export interface TitleConnection {
  kind: "local" | "app" | "ssh"
  // label is the ssh destination; unused for local and app.
  label: string
  ready: boolean
}

// windowTitle: local → page; ssh → "<page> - <label> (SSH)", or
// "<page> - <label> (SSH, reconnecting)" while not ready; app → "servef - Connect".
export function windowTitle(page: string, connection: TitleConnection): string {
  switch (connection.kind) {
    case "local":
      return page
    case "app":
      return "servef - Connect"
    case "ssh":
      return `${page} - ${connection.label} (SSH${connection.ready ? "" : ", reconnecting"})`
  }
}
