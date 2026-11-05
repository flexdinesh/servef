const APP_ORIGIN = "servef-app://"

// shareableOrigin is the origin to put in copied text, or null when the page
// has no address another program can use. The desktop app's servef-app://local
// shares as servef://local, which opens the app. Any other origin of that
// scheme has none. Origins outside the app are unchanged.
export function shareableOrigin(origin: string): string | null {
  if (!origin.startsWith(APP_ORIGIN)) return origin
  return origin === `${APP_ORIGIN}local` ? "servef://local" : null
}
