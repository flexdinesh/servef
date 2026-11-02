// protocol adapter for the servef-app:// scheme (decision 0067).
import { protocol } from "electron"
import { APP_SCHEME } from "./bridge.ts"

// registerAppScheme must run before the app is ready.
export function registerAppScheme(): void {
  protocol.registerSchemesAsPrivileged([{
    scheme: APP_SCHEME,
    privileges: { standard: true, secure: true, supportFetchAPI: true, stream: true, codeCache: true },
  }])
}

// handleAppScheme serves every servef-app:// request with handler.
export function handleAppScheme(handler: (request: Request) => Promise<Response>): void {
  protocol.handle(APP_SCHEME, handler)
}
