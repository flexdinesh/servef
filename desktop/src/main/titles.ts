// Pure window titles (decisions 0070, 0071, 0075).

export interface TitleConnection {
  kind: "local" | "app" | "ssh" | "cli" | "http" | "ssh-port"
  // label is the ssh destination, localhost:<port>, <host>:<port>, or
  // <destination>:<port>; unused for local and app.
  label: string
  ready: boolean
}

// windowTitle: local and app pages (whose titles start with "servef - ")
// → page; ssh → "<page> - <label> (SSH)", or "<page> - <label> (SSH,
// reconnecting)" while not ready; ssh-port → "<page> - <label> (SSH)"; cli
// and http → "<page> - <label>".
export function windowTitle(page: string, connection: TitleConnection): string {
  switch (connection.kind) {
    case "local":
    case "app":
      return page
    case "ssh":
      return `${page} - ${connection.label} (SSH${connection.ready ? "" : ", reconnecting"})`
    case "ssh-port":
      return `${page} - ${connection.label} (SSH)`
    case "cli":
    case "http":
      return `${page} - ${connection.label}`
  }
}
