import assert from "node:assert/strict"
import test from "node:test"

import { formatFailureMessage } from "./document-format.ts"
import { documentModes, documentView, modeDisabledReason, resolveDocumentMode } from "./document-view.ts"
import type { FileType, PageData } from "../../page-contract.ts"

const emptyPage: PageData = {
  roots: ["/root"],
  branch: "",
  project: ".",
  projectUnavailable: false,
  filenamePatterns: ["*"], documentType: null, navigationSupported: false, binary: false, tooLarge: false, previewDisabled: false, searchExcluded: false,
  highlightDisabled: false, previewError: "", content: "", empty: false, error: "", fileCount: 0, fileSize: 0,
  hasFile: false, rootName: "", repository: "", projectKind: "folder", worktrees: 0, selected: "", source: "", tree: [], warnings: [],
}

function fileType(overrides: Partial<FileType>): FileType {
  return {
    id: "text", label: "Plain text", extensions: [], filenames: [], language: "plaintext",
    previewKind: "", previewLabel: "", defaultMode: "source", formattable: false, ...overrides,
  }
}

function page(documentType: FileType, overrides: Partial<PageData> = {}): PageData {
  return { ...emptyPage, hasFile: true, selected: "file", fileSize: 6_500_000, documentType, ...overrides }
}

const markdown = fileType({ id: "markdown", previewKind: "markdown", previewLabel: "Preview" })
const json = fileType({ id: "json", label: "JSON", formattable: true })
const svg = fileType({ id: "svg", previewKind: "image", previewLabel: "Preview", defaultMode: "preview", language: "xml" })
const raster = fileType({ id: "image", previewKind: "image", previewLabel: "Preview", defaultMode: "preview", language: "" })

test("declared capabilities select toolbar modes", () => {
  assert.deepEqual(documentModes(page(markdown)), ["source", "preview"])
  assert.deepEqual(documentModes(page(json)), ["source", "format"])
  assert.deepEqual(documentModes(page(svg)), ["preview", "source"])
  assert.deepEqual(documentModes(page(raster, { binary: true })), ["preview"])
  assert.deepEqual(documentModes(page(fileType({}))), ["source"])
  assert.deepEqual(documentModes(page(fileType({}), { binary: true })), [])
  assert.deepEqual(documentModes(page(markdown, { binary: true })), [])
  assert.deepEqual(documentModes(emptyPage), [])
  assert.deepEqual(documentModes(page(markdown, { tooLarge: true, previewDisabled: true })), [])
  assert.deepEqual(documentModes(page(json, { tooLarge: true, previewDisabled: true })), [])
})

test("modes resolve from request, then default, and respect large-file limits", () => {
  assert.equal(resolveDocumentMode(page(markdown), undefined), "source")
  assert.equal(resolveDocumentMode(page(markdown), "preview"), "preview")
  assert.equal(resolveDocumentMode(page(svg), undefined), "preview")
  assert.equal(resolveDocumentMode(page(json), undefined), "source")
  assert.equal(resolveDocumentMode(page(json), "format"), "format")
  assert.equal(resolveDocumentMode(page(json), "preview"), "source")
  assert.equal(resolveDocumentMode(page(markdown, { previewDisabled: true }), "preview"), "source")
  assert.equal(resolveDocumentMode(page(json, { previewDisabled: true }), "format"), "source")
  assert.equal(resolveDocumentMode(page(fileType({}), { binary: true }), undefined), null)
  assert.equal(resolveDocumentMode(page(markdown, { binary: true }), "preview"), null)
  assert.equal(modeDisabledReason(page(json, { previewDisabled: true }), "format"), "Large file (6.2 MB): formatting is off")
  assert.equal(modeDisabledReason(page(json, { previewDisabled: true }), "source"), "")
  assert.equal(modeDisabledReason(page(markdown), "preview"), "")
})

test("format results replace Source only once ready", () => {
  const json = page(fileType({ formattable: true }))
  assert.deepEqual(documentView(json, "format", undefined), { kind: "source", formatPending: true, formatError: "" })
  assert.deepEqual(documentView(json, "format", { kind: "loading" }), { kind: "source", formatPending: true, formatError: "" })
  assert.deepEqual(documentView(json, "format", { kind: "ready", text: "{}\n" }), { kind: "format", text: "{}\n" })
  assert.deepEqual(documentView(json, "source", { kind: "error", message: "bad" }), { kind: "source", formatPending: false, formatError: "bad" })
  assert.deepEqual(documentView(page(svg), "preview", undefined), { kind: "image" })
  assert.deepEqual(documentView(page(markdown), "preview", undefined), { kind: "preview" })
  assert.deepEqual(documentView(page(fileType({}), { binary: true }), null, undefined), { kind: "binary" })
  const binaryMarkdown = page(markdown, { binary: true })
  assert.deepEqual(documentView(binaryMarkdown, resolveDocumentMode(binaryMarkdown, undefined), undefined), { kind: "binary" })
  assert.deepEqual(documentView(emptyPage, null, undefined), { kind: "none" })
})

test("files over the display cap show the too-large note instead of any view", () => {
  for (const type of [markdown, json, fileType({})]) {
    const huge = page(type, { tooLarge: true, previewDisabled: true, searchExcluded: true })
    assert.equal(resolveDocumentMode(huge, undefined), null)
    assert.equal(resolveDocumentMode(huge, "source"), null)
    assert.deepEqual(documentView(huge, resolveDocumentMode(huge, "source"), undefined), { kind: "tooLarge" })
  }
  assert.deepEqual(documentView(page(fileType({}), { binary: true }), null, undefined), { kind: "binary" })
})

test("format failures read as one sentence", () => {
  assert.equal(formatFailureMessage("invalid character '}' at line 3."), "Couldn't format: invalid character '}' at line 3.")
  assert.equal(formatFailureMessage(""), "Couldn't format: the server returned an invalid response.")
})
