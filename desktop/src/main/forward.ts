import http from "node:http"
import { responseHeaders } from "./headers.ts"

// Forward sends one request over the daemon's Unix socket and resolves with
// a streamed web Response. It rejects only when the request could not be
// sent or no response head arrived (see isDaemonGone). Cancelling the
// response body destroys the upstream request.
export type Forward = (request: ForwardRequest) => Promise<Response>

export interface ForwardRequest {
  socket: string
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

// createForwarder keeps a keep-alive agent (maxSockets 16) for ordinary requests.
// Listeners attach to requests and responses only, never to pooled sockets.
export function createForwarder(): Forwarder {
  const agent = new http.Agent({ keepAlive: true, maxSockets: 16 })
  return {
    forward: (request) => send(request, request.stream ? false : agent),
    close: () => agent.destroy(),
  }
}

// isDaemonGone reports a socket connect failure (ECONNREFUSED, ENOENT).
export function isDaemonGone(error: unknown): boolean {
  return typeof error === "object" && error !== null && "code" in error
    && (error.code === "ECONNREFUSED" || error.code === "ENOENT")
}

function send(request: ForwardRequest, agent: http.Agent | false): Promise<Response> {
  return new Promise((resolve, reject) => {
    const signal = request.signal
    if (signal?.aborted) {
      reject(abortError(signal))
      return
    }
    const upstream = http.request({ socketPath: request.socket, method: request.method, path: request.path, headers: request.headers, agent })
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
