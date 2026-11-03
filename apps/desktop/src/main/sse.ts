// createSSEParser parses a text/event-stream incrementally. Comments are
// skipped; events without a name are "message"; multiline data joins with "\n".
// Lines end in CRLF, LF, or CR (a CR may end one chunk and its LF start the
// next). id and retry fields are ignored.
export function createSSEParser(onEvent: (name: string, data: string) => void): (chunk: string) => void {
  let buffer = ""
  let skipLF = false
  let first = true
  let name = ""
  let data: string[] = []

  const line = (text: string) => {
    if (text === "") {
      if (data.length > 0) onEvent(name === "" ? "message" : name, data.join("\n"))
      name = ""
      data = []
      return
    }
    if (text.startsWith(":")) return
    const colon = text.indexOf(":")
    const field = colon < 0 ? text : text.slice(0, colon)
    let value = colon < 0 ? "" : text.slice(colon + 1)
    if (value.startsWith(" ")) value = value.slice(1)
    if (field === "event") name = value
    else if (field === "data") data.push(value)
  }

  return (chunk) => {
    let text = chunk
    if (text === "") return
    if (first) {
      first = false
      if (text.startsWith("﻿")) text = text.slice(1)
    }
    if (skipLF) {
      skipLF = false
      if (text.startsWith("\n")) text = text.slice(1)
    }
    buffer += text
    let start = 0
    for (let index = 0; index < buffer.length; index++) {
      const char = buffer[index]
      if (char !== "\n" && char !== "\r") continue
      line(buffer.slice(start, index))
      if (char === "\r") {
        if (index + 1 === buffer.length) skipLF = true
        else if (buffer[index + 1] === "\n") index++
      }
      start = index + 1
    }
    buffer = buffer.slice(start)
  }
}
