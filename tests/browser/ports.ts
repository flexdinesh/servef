// Browser-test servers listen on PORT (default 18080) and the next two ports.
function basePort(value: string | undefined): number {
  if (value === undefined || value === "") return 18080
  const port = Number(value)
  if (!Number.isInteger(port) || port < 1 || port > 65533) throw new Error(`PORT must be a port number up to 65533, got ${JSON.stringify(value)}`)
  return port
}

export const apiPort = basePort(process.env.PORT)
export const tldrawApiPort = apiPort + 1
export const filteredApiPort = apiPort + 2

export const apiOrigin = `http://127.0.0.1:${apiPort}`
export const tldrawApiOrigin = `http://127.0.0.1:${tldrawApiPort}`
export const filteredApiOrigin = `http://127.0.0.1:${filteredApiPort}`
