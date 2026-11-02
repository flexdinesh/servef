const appOrigin = "servef-app://"

// shareableOrigin is the origin to put in copied text. Desktop app pages share
// as servef:// links, which open the app: servef-app://local as
// servef://local, servef-app://ssh-<slug> as servef://ssh/<slug> (the app
// resolves the slug to the known host). A CLI server window,
// servef-app://cli-<port>, shares its own http://localhost:<port> address.
// Any other origin is unchanged.
export function shareableOrigin(origin: string): string {
  if (!origin.startsWith(appOrigin)) return origin
  const host = origin.slice(appOrigin.length)
  if (host === "local") return "servef://local"
  const port = /^cli-([1-9][0-9]{0,4})$/.exec(host)?.[1]
  if (port !== undefined && Number(port) <= 65535) return `http://localhost:${port}`
  const slug = /^ssh-([a-z0-9][a-z0-9-]*)$/.exec(host)?.[1]
  return slug === undefined ? origin : `servef://ssh/${slug}`
}
