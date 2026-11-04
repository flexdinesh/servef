package contract

// Source control error codes (decisions 0024, 0030, 0040).
const (
	// CodeUnchanged marks a not-found diff of a path with no changes in the
	// compare.
	CodeUnchanged = "unchanged"
	// CodeUnknownBase marks an invalid base branch /api/refs does not list.
	CodeUnknownBase = "unknownBase"
	// CodeUnknownHead marks an invalid head that is not a local branch
	// /api/refs lists.
	CodeUnknownHead = "unknownHead"
)

// Code navigation error codes (decision 0018): every navigation failure
// carries one.
const (
	// CodeNavigationUnavailable: the project or file is unavailable or
	// cannot navigate.
	CodeNavigationUnavailable = "unavailable"
	// CodeNavigationInvalid: no file, or an invalid line or column.
	CodeNavigationInvalid = "invalid"
	// CodeNavigationUnsupported: no language server serves the file.
	CodeNavigationUnsupported = "unsupported"
	// CodeNavigationDisabled: the request did not authorize language servers.
	CodeNavigationDisabled = "disabled"
	// CodeNavigationMissing: the language server is not installed.
	CodeNavigationMissing = "missing"
	// CodeNavigationStarting: the language server is still starting.
	CodeNavigationStarting = "starting"
	// CodeNavigationTimeout: the language server did not answer in time.
	CodeNavigationTimeout = "timeout"
	// CodeNavigationUnsupportedMethod: the server lacks the method.
	CodeNavigationUnsupportedMethod = "unsupported-method"
	// CodeNavigationFailed: the language server failed.
	CodeNavigationFailed = "failed"
)

// Walkthrough error codes (decision 0047); a missing walkthrough is
// CodeWalkthroughUnavailable.
const (
	// CodeInvalidWalkthrough: the draft is not strict JSON, is malformed, or
	// names targets the project lacks.
	CodeInvalidWalkthrough = "invalidWalkthrough"
	// CodeWalkthroughTooLarge: the draft is over 1 MiB encoded.
	CodeWalkthroughTooLarge = "tooLarge"
	// CodeForeignOrigin: a browser on another site sent the write.
	CodeForeignOrigin = "foreignOrigin"
)
