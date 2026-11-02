// Packages the desktop app for the host OS, one arch per run (decision 0068):
//   node desktop/scripts/package.ts --arch amd64|arm64 --servef <binary> [--version <x.y.z>] [--dir]
// darwin: dmg + zip; linux: AppImage + deb; --dir: unpacked app. Output in desktop/dist/.
import { execFileSync } from "node:child_process"
import { chmodSync, copyFileSync, mkdirSync, statSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { parseArgs } from "node:util"

import { Arch, build, Platform } from "electron-builder"

const desktop = fileURLToPath(new URL("..", import.meta.url))
const USAGE = "usage: node desktop/scripts/package.ts --arch amd64|arm64 --servef <binary> [--version <x.y.z>] [--dir]"
// Semantic version, as electron-builder requires.
const VERSION = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/

interface Host {
  os: "darwin" | "linux"
  platform: Platform
  targets: string[]
}

function fail(message: string): never {
  console.error(`package: ${message}\n${USAGE}`)
  process.exit(2)
}

function flags() {
  try {
    return parseArgs({
      options: {
        arch: { type: "string" },
        servef: { type: "string" },
        version: { type: "string" },
        dir: { type: "boolean", default: false },
      },
      strict: true,
      allowPositionals: false,
    }).values
  } catch (error: unknown) {
    fail(error instanceof Error ? error.message : String(error))
  }
}

function parse(): { arch: "amd64" | "arm64"; servef: string; version: string | undefined; dir: boolean } {
  const { arch, servef, version, dir } = flags()
  if (arch !== "amd64" && arch !== "arm64") fail(`--arch must be amd64 or arm64, got ${arch ?? "nothing"}`)
  if (servef === undefined || servef === "") fail("--servef <binary> is required")
  if (version !== undefined && !VERSION.test(version)) fail(`--version must be a semantic version like 1.2.3, got ${version}`)
  return { arch, servef: path.resolve(servef), version, dir }
}

function host(): Host {
  if (process.platform === "darwin") return { os: "darwin", platform: Platform.MAC, targets: ["dmg", "zip"] }
  if (process.platform === "linux") return { os: "linux", platform: Platform.LINUX, targets: ["AppImage", "deb"] }
  fail(`unsupported host OS ${process.platform}: package on macOS or Linux`)
}

async function main(): Promise<void> {
  const args = parse()
  const target = host()
  try {
    if (!statSync(args.servef).isFile()) fail(`--servef ${args.servef} is not a file`)
  } catch (error: unknown) {
    fail(`--servef ${args.servef}: ${error instanceof Error ? error.message : String(error)}`)
  }

  // electron-builder.yml copies .build/bin/servef into the app's resources/bin.
  const bin = path.join(desktop, ".build", "bin")
  mkdirSync(bin, { recursive: true })
  const binary = path.join(bin, "servef")
  copyFileSync(args.servef, binary)
  chmodSync(binary, 0o755)

  execFileSync(process.execPath, ["build.ts"], { cwd: desktop, stdio: "inherit" })

  // electron-builder's ${os} and ${arch} macros say mac and x64, so name per run.
  const artifactName = `servef-desktop_\${version}_${target.os}_${args.arch}.\${ext}`
  const artifacts = await build({
    projectDir: desktop,
    targets: target.platform.createTarget(args.dir ? "dir" : target.targets, args.arch === "amd64" ? Arch.x64 : Arch.arm64),
    publish: "never",
    config: {
      artifactName,
      ...(args.version === undefined ? {} : { extraMetadata: { version: args.version } }),
    },
  })
  for (const artifact of artifacts) console.log(path.relative(process.cwd(), artifact))
}

main().catch((error: unknown) => {
  console.error("package: electron-builder failed:", error)
  process.exit(1)
})
