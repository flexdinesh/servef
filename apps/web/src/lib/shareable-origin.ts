const APP_ORIGIN = "servef-app://"
const CONNECTION = /^(?:local|r-[0-9a-f]{12})$/

// shareableOrigin is the origin to put in copied text, or null when the page
// has no address another program can use. The desktop app's servef-app://local
// and servef-app://<connection id> share as servef://…, which opens the app on
// a computer with that connection saved. Any other origin of that scheme has
// none. Origins outside the app are unchanged.
export function shareableOrigin(origin: string): string | null {
  if (!origin.startsWith(APP_ORIGIN)) return origin
  const connection = origin.slice(APP_ORIGIN.length)
  return CONNECTION.test(connection) ? `servef://${connection}` : null
}
