package contract

// ErrorResponse is every JSON error body. Code, when set, names a failure
// clients handle specially.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}
