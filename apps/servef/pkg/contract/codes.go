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
