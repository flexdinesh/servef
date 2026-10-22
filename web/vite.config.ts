import { readFile } from "node:fs/promises"
import { networkInterfaces } from "node:os"
import { join } from "node:path"
import { fileURLToPath, URL } from "node:url"

import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig, type Plugin } from "vite"

import {
  devServerUrls,
  formatDevServerUrl,
  mockFixtureName,
  primaryLanIpv4,
  shouldOpenBrowser,
} from "./dev-server.ts"

const devServerBanner: Plugin = {
  name: "servef-dev-server-banner",
  configureServer(server) {
    server.printUrls = () => {
      const address = server.httpServer?.address()

      if (address === null || address === undefined || typeof address === "string") {
        return
      }

      const lanAddress = primaryLanIpv4(networkInterfaces())
      for (const url of devServerUrls(address.port, lanAddress)) {
        server.config.logger.info(formatDevServerUrl(url))
      }
    }
  },
}

// @pierre/diffs can load Shiki, its grammars, and every Pierre, Shiki, and
// Highlights theme. servef highlights only with Highlights and its own theme,
// so production builds resolve those imports to local stand-ins and none of
// them ship (decision 0029).
const shikiUnavailable = "./src/shiki-unavailable.ts"
const diffsSubsets: ReadonlyMap<string, string> = new Map([
  ["shiki", shikiUnavailable],
  ["shiki/core", shikiUnavailable],
  ["shiki/langs", shikiUnavailable],
  ["shiki/textmate", shikiUnavailable],
  ["shiki/wasm", shikiUnavailable],
  ["shiki/engine/javascript", shikiUnavailable],
  ["shiki/engine/oniguruma", shikiUnavailable],
  ["@pierre/theming/themes", "./src/pierre-themes-subset.ts"],
  ["@pierre/highlights/themes/loader", "./src/highlights-themes-subset.ts"],
].map(([source, subset]) => [source, fileURLToPath(new URL(subset, import.meta.url))]))

const diffsSubset: Plugin = {
  name: "servef-diffs-subset",
  apply: "build",
  enforce: "pre",
  resolveId(source, importer) {
    if (importer === undefined || !importer.replaceAll("\\", "/").includes("/@pierre/diffs/")) return null
    return diffsSubsets.get(source) ?? null
  },
}

const mockFixturesDirectory = fileURLToPath(
  new URL("../testdata/api/default", import.meta.url),
)

function mockApi(enabled: boolean): Plugin {
  return {
    name: "servef-mock-api",
    apply: "serve",
    configureServer(server) {
      if (!enabled) return
      server.middlewares.use("/api", async (request, response, next) => {
        const fixture = mockFixtureName(request.method ?? "", request.url ?? "")
        if (fixture === undefined) {
          next()
          return
        }

        try {
          response.statusCode = fixture.status
          response.setHeader("Cache-Control", "no-store")
          response.setHeader("Content-Type", fixture.contentType ?? "application/json; charset=utf-8")
          response.end(await readFile(join(mockFixturesDirectory, fixture.name)))
        } catch {
          response.statusCode = 500
          response.end('{"error":"mock fixture unavailable"}')
        }
      })
    },
  }
}

export default defineConfig(({ command, mode }) => ({
  base: command === "build" ? "/assets/" : "/",
  build: {
    assetsDir: "",
    emptyOutDir: true,
    outDir: "../internal/web/dist",
  },
  plugins: [diffsSubset, react(), tailwindcss(), devServerBanner, mockApi(mode === "mock")],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  root: fileURLToPath(new URL(".", import.meta.url)),
  server: {
    allowedHosts: true,
    host: "0.0.0.0",
    open: shouldOpenBrowser(process.env),
    proxy: mode === "mock" ? undefined : {
      "/api": process.env.SERVEF_API_TARGET || "http://127.0.0.1:8080",
    },
  },
  worker: {
    plugins: () => [diffsSubset],
  },
}))
