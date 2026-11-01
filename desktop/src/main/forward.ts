import http from "node:http"
import { responseHeaders } from "./headers.ts"

// Forward sends one request to a daemon's Unix socket or a CLI server's TCP
// port (decision 0071) and resolves with
// a streamed web Response. It rejects only when the request could not be
// sent or no response head arrived (see isDaemonGone). Cancelling the
// response body destroys the upstream request.
export type Forward = (request: ForwardRequest) => Promise<Response>

// ForwardTarget is a Unix socket (local daemon, ssh forward) or a loopback
// TCP address (a CLI server).
export type ForwardTarget = { socket: string } | { host: string; port: number }

export interface ForwardRequest {
  target: ForwardTarget
  method: string
  // path is the request target: pathname plus search, already encoded.
  path: string
  headers: Record<string, string>
  body: ReadableStream<Uint8Array> | null
  // stream requests (text/event-stream) get their own socket (agent: false).
  stream: boolean
  signal?: AbortSignal
}

export interface Forwarder {
  forward: Forward
  close(): void
}

// createForwarder keeps one keep-alive agent (maxSockets 16) per target for
// ordinary requests. Listeners attach to requests and responses only, never
// to pooled sockets.
export function createForwarder(): Forwarder {
  const agents = new Map<string, http.Agent>()
  const agentFor = (target: ForwardTarget): http.Agent => {
    const key = targetKey(target)
    let agent = agents.get(key)
    if (agent === undefined) {
      agent = new http.Agent({ keepAlive: true, maxSockets: 16 })
      agents.set(key, agent)
    }
    return agent
  }
  return {
    forward: (request) => send(request, request.stream ? false : agentFor(request.target)),
    close: () => {
      for (const agent of agents.values()) agent.destroy()
      agents.clear()
    },
  }
}

function targetKey(target: ForwardTarget): string {
  return "socket" in target ? `unix:${target.socket}` : `tcp:${target.host}:${target.port}`
}

// isDaemonGone reports a socket failure before a response head: connect
// refused or missing (ECONNREFUSED, ENOENT), a reset (ECONNRESET, which
// Node's "socket hang up" carries), or a close while the request was written
// (EPIPE: ssh closes a forwarded connection the server refused), so the
// caller re-ensures.
export function isDaemonGone(error: unknown): boolean {
  return typeof error === "object" && error !== null && "code" in error
    && (error.code === "ECONNREFUSED" || error.code === "ENOENT" || error.code === "ECONNRESET" || error.code === "EPIPE")
}

function send(request: ForwardRequest, agent: http.Agent | false): Promise<Response> {
  return new Promise((resolve, reject) => {
    const signal = request.signal
    if (signal?.aborted) {
      reject(abortError(signal))
      return
    }
    const where = "socket" in request.target
      ? { socketPath: request.target.socket }
      // autoSelectFamily tries both families for localhost (::1 and 127.0.0.1).
      : { host: request.target.host, port: request.target.port, autoSelectFamily: true }
    const upstream = http.request({ ...where, method: request.method, path: request.path, headers: request.headers, agent })
    // The signal is a backstop; renderer cancellation arrives as body cancel.
    const onAbort = () => { if (signal !== undefined) upstream.destroy(abortError(signal)) }
    signal?.addEventListener("abort", onAbort)
    const finish = () => signal?.removeEventListener("abort", onAbort)
    upstream.on("error", (error) => {
      // After a response head the body stream reports failures instead.
      finish()
      reject(error)
    })
    upstream.on("response", (res) => {
      const init = { status: res.statusCode ?? 502, statusText: res.statusMessage ?? "", headers: responseHeaders(res.headers) }
      if (request.method === "HEAD" || init.status === 204 || init.status === 304) {
        res.on("close", finish)
        res.resume()
        resolve(new Response(null, init))
        return
      }
      resolve(new Response(responseBody(res, upstream, finish), init))
    })
    if (request.body === null) upstream.end()
    else void pumpBody(request.body, upstream)
  })
}

// responseBody adapts an IncomingMessage to a web stream with backpressure;
// cancel destroys the upstream request so the daemon sees the client leave.
function responseBody(res: http.IncomingMessage, upstream: http.ClientRequest, finish: () => void): ReadableStream<Uint8Array> {
  let done = false
  const settle = (): boolean => {
    if (done) return false
    done = true
    finish()
    return true
  }
  return new ReadableStream<Uint8Array>({
    start(controller) {
      res.on("data", (chunk: Buffer) => {
        if (done) return
        controller.enqueue(new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength))
        if ((controller.desiredSize ?? 1) <= 0) res.pause()
      })
      res.on("end", () => { if (settle()) controller.close() })
      res.on("error", (error) => { if (settle()) controller.error(error) })
      res.on("close", () => { if (settle()) controller.error(new Error("The servef daemon closed the response early.")) })
    },
    pull() {
      res.resume()
    },
    cancel() {
      settle()
      upstream.destroy()
    },
  })
}

// pumpBody streams a request body upstream, honoring backpressure.
async function pumpBody(body: ReadableStream<Uint8Array>, upstream: http.ClientRequest): Promise<void> {
  const reader = body.getReader()
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      if (upstream.destroyed) {
        await reader.cancel()
        return
      }
      if (!upstream.write(value)) await drained(upstream)
    }
    upstream.end()
  } catch (error) {
    upstream.destroy(error instanceof Error ? error : new Error(String(error)))
  }
}

function drained(upstream: http.ClientRequest): Promise<void> {
  return new Promise((resolve) => {
    const done = () => {
      upstream.off("drain", done)
      upstream.off("close", done)
      resolve()
    }
    upstream.on("drain", done)
    upstream.on("close", done)
  })
}

function abortError(signal: AbortSignal): Error {
  return signal.reason instanceof Error ? signal.reason : new DOMException("The request was aborted.", "AbortError")
}
