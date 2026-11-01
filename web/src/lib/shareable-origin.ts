const appOrigin = "servef-app://"

// shareableOrigin is the origin to put in copied text: the desktop app's
// servef-app:// pages share as servef:// links, which open the app; any
// other origin is already shareable.
export function shareableOrigin(origin: string): string {
  return origin.startsWith(appOrigin) ? `servef://${origin.slice(appOrigin.length)}` : origin
}
