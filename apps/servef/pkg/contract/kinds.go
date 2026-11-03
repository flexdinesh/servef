package contract

import "net/http"

// ErrorKind classifies an application failure independently of transport.
// HTTP maps each kind to one status; other transports carry the kind itself.
type ErrorKind string

const (
	// KindInvalid: the request is malformed or names something it may not.
	KindInvalid ErrorKind = "invalid"
	// KindNotFound: the project, file, or path is unavailable.
	KindNotFound ErrorKind = "notFound"
	// KindForbidden: the path exists but cannot be read.
	KindForbidden ErrorKind = "forbidden"
	// KindTooLarge: the file exceeds the operation's size limit.
	KindTooLarge ErrorKind = "tooLarge"
	// KindUnsupported: the file's media type is not served.
	KindUnsupported ErrorKind = "unsupported"
	// KindUnprocessable: the content cannot be processed (binary, malformed).
	KindUnprocessable ErrorKind = "unprocessable"
	// KindUnavailable: a server capability is not available now.
	KindUnavailable ErrorKind = "unavailable"
	// KindInternal: an unexpected failure.
	KindInternal ErrorKind = "internal"
)

var kindStatuses = map[ErrorKind]int{
	KindInvalid:       http.StatusBadRequest,
	KindNotFound:      http.StatusNotFound,
	KindForbidden:     http.StatusForbidden,
	KindTooLarge:      http.StatusRequestEntityTooLarge,
	KindUnsupported:   http.StatusUnsupportedMediaType,
	KindUnprocessable: http.StatusUnprocessableEntity,
	KindUnavailable:   http.StatusServiceUnavailable,
	KindInternal:      http.StatusInternalServerError,
}

// HTTPStatus is the status HTTP answers for kind; unknown kinds are 500.
func HTTPStatus(kind ErrorKind) int {
	if status, ok := kindStatuses[kind]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// KindForStatus inverts HTTPStatus for a failed HTTP answer; statuses no kind
// maps to are KindInternal.
func KindForStatus(status int) ErrorKind {
	for kind, mapped := range kindStatuses {
		if mapped == status {
			return kind
		}
	}
	return KindInternal
}
