import { readdir, readFile } from "node:fs/promises"
import { networkInterfaces } from "node:os"
import { isAbsolute, join, relative, resolve } from "node:path"
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

const excalidrawFontsDirectory = fileURLToPath(
  new URL(
    "./node_modules/@excalidraw/excalidraw/dist/prod/fonts",
    import.meta.url,
  ),
)

async function listFiles(directory: string, relativeDirectory = ""): Promise<string[]> {
  const entries = await readdir(join(directory, relativeDirectory), {
    withFileTypes: true,
  })
  const files = await Promise.all(
    entries.map(async (entry) => {
      const relativePath = relativeDirectory
        ? `${relativeDirectory}/${entry.name}`
        : entry.name

      if (entry.isDirectory()) {
        return listFiles(directory, relativePath)
      }
      return entry.isFile() ? [relativePath] : []
    }),
  )

  return files.flat().sort()
}

const excalidrawFonts: Plugin = {
  name: "servef-excalidraw-fonts",
  apply: "build",
  async generateBundle() {
    for (const relativePath of await listFiles(excalidrawFontsDirectory)) {
      this.emitFile({
        type: "asset",
        fileName: `excalidraw/fonts/${relativePath}`,
        source: await readFile(join(excalidrawFontsDirectory, relativePath)),
      })
    }
  },
}

const devFontAssets: Plugin = {
  name: "servef-dev-font-assets",
  apply: "serve",
  configureServer(server) {
    server.middlewares.use("/assets/excalidraw/fonts", async (request, response, next) => {
      try {
        const requestPath = request.url?.split("?", 1)[0] ?? ""
        const relativePath = decodeURIComponent(requestPath).replace(/^\/+/, "")
        const candidate = resolve(excalidrawFontsDirectory, relativePath)
        const fromRoot = relative(excalidrawFontsDirectory, candidate)
        if (!relativePath || isAbsolute(fromRoot) || fromRoot.startsWith("..")) {
          next()
          return
        }

        const content = await readFile(candidate)
        response.setHeader("Content-Type", "font/woff2")
        response.end(content)
      } catch {
        next()
      }
    })
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
  optimizeDeps: {
    include: ["minisearch"],
  },
  plugins: [react(), tailwindcss(), devServerBanner, devFontAssets, mockApi(mode === "mock"), excalidrawFonts],
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
}))
