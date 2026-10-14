export interface NetworkAddress {
  address: string
  family: string
  internal: boolean
}

export type NetworkInterfaces = Readonly<
  Record<string, readonly NetworkAddress[] | undefined>
>

export type Environment = Readonly<Record<string, string | undefined>>

export interface DevServerUrl {
  label: "Local" | "Host" | "Network"
  url: string
}

export interface MockFixture {
  name: string
  status: number
  contentType?: string
}

// Maps keep request paths such as "constructor" from hitting prototype keys.
const mockPages: ReadonlyMap<string, string> = new Map([
  ["assets/preview.png", "page-image.json"],
  ["assets/sample.bin", "page-binary.json"],
  ["config/broken.json", "page-json-broken.json"],
  ["config/settings.json", "page-json.json"],
  ["guides/getting-started.md", "page-document.json"],
  ["logs/enormous.log", "page-too-large.json"],
  ["logs/huge.log", "page-large.json"],
])

const mockFormats: ReadonlyMap<string, MockFixture> = new Map([
  ["config/broken.json", { name: "format-error.json", status: 422 }],
  ["config/settings.json", { name: "format-settings.json", status: 200 }],
])

const mockRaw: ReadonlyMap<string, MockFixture> = new Map([
  ["assets/preview.png", { name: "raw-preview.png", status: 200, contentType: "image/png" }],
])

const notFound: MockFixture = { name: "error-not-found.json", status: 404 }

export function primaryLanIpv4(
  interfaces: NetworkInterfaces,
): string | undefined {
  for (const addresses of Object.values(interfaces)) {
    for (const candidate of addresses ?? []) {
      if (candidate.family === "IPv4" && !candidate.internal) {
        return candidate.address
      }
    }
  }

  return undefined
}

export function devServerUrls(
  port: number,
  lanAddress: string | undefined,
): readonly DevServerUrl[] {
  const urls: DevServerUrl[] = [
    { label: "Local", url: `http://localhost:${port}/` },
    { label: "Host", url: `http://0.0.0.0:${port}/` },
  ]

  if (lanAddress !== undefined) {
    urls.push({ label: "Network", url: `http://${lanAddress}:${port}/` })
  }

  return urls
}

export function formatDevServerUrl({ label, url }: DevServerUrl): string {
  return `  ➜  ${label}:${" ".repeat(8 - label.length)}${url}`
}

export function isSshSession(environment: Environment): boolean {
  return ["SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"].some(
    (name) => Boolean(environment[name]),
  )
}

export function shouldOpenBrowser(environment: Environment): boolean {
  return ["dev", "dev:mock"].includes(environment.npm_lifecycle_event ?? "")
    && !isSshSession(environment)
}

export function mockFixtureName(method: string, rawURL: string): MockFixture | undefined {
  if (method !== "GET") return undefined
  const url = new URL(rawURL, "http://localhost")
  switch (url.pathname) {
    case "/features":
      return { name: "features.json", status: 200 }
    case "/metrics":
      return { name: "metrics.json", status: 200 }
    case "/search-documents":
      return { name: "search-documents.json", status: 200 }
    case "/projects":
      return { name: "projects.json", status: 200 }
    case "/directories":
      return { name: "directories.json", status: 200 }
    case "/page": {
      const path = url.searchParams.get("path")
      if (path === null) return { name: "page-index.json", status: 200 }
      const page = mockPages.get(path)
      return page ? { name: page, status: 200 } : { name: "page-missing.json", status: 404 }
    }
    case "/format":
      return mockFormats.get(url.searchParams.get("path") ?? "") ?? notFound
    case "/raw":
      return mockRaw.get(url.searchParams.get("path") ?? "") ?? notFound
    default:
      return undefined
  }
}
