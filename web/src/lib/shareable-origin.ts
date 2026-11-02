const appOrigin = "servef-app://"

const OCTET = "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
const PORT = "([1-9][0-9]{0,4})"
// HTTP_IPV4 matches servef-app://http-<a>-<b>-<c>-<d>-<port>: the desktop app
// gives only an IPv4 server a four-octet slug (decision 0075).
const HTTP_IPV4 = new RegExp(`^http-${OCTET}-${OCTET}-${OCTET}-${OCTET}-${PORT}$`)
const CLI = new RegExp(`^cli-${PORT}$`)

// shareableOrigin is the origin to put in copied text, or null when the page
// has no address another program can use. servef-app://local shares as
// servef://local, which opens the app. A CLI server window,
// servef-app://cli-<port>, shares its own http://localhost:<port> address,
// and an http server window on an IPv4 address its http://<address>:<port>.
// ssh windows, other http servers, and other app origins have none: the app
// alone knows how it reaches them. Origins outside the app are unchanged.
export function shareableOrigin(origin: string): string | null {
  if (!origin.startsWith(appOrigin)) return origin
  const host = origin.slice(appOrigin.length)
  if (host === "local") return "servef://local"
  const cli = CLI.exec(host)
  if (cli !== null) return validPort(cli[1]) ? `http://localhost:${cli[1]}` : null
  const http = HTTP_IPV4.exec(host)
  if (http === null || !validPort(http[5])) return null
  return `http://${http[1]}.${http[2]}.${http[3]}.${http[4]}:${http[5]}`
}

function validPort(digits: string | undefined): boolean {
  return digits !== undefined && Number(digits) <= 65535
}
