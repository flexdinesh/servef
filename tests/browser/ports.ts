// Browser-test servers listen on PORT (default 18080) and the next port.
function basePort(value: string | undefined): number {
  if (value === undefined || value === "") return 18080
  const port = Number(value)
  if (!Number.isInteger(port) || port < 1 || port > 65534) throw new Error(`PORT must be a port number up to 65534, got ${JSON.stringify(value)}`)
  return port
}

export const apiPort = basePort(process.env.PORT)
export const filteredApiPort = apiPort + 1

export const apiOrigin = `http://127.0.0.1:${apiPort}`
export const filteredApiOrigin = `http://127.0.0.1:${filteredApiPort}`
