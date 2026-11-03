package contract

import (
	"net/http"
	"testing"
)

func TestErrorKindsMapToDistinctStatuses(t *testing.T) {
	seen := map[int]ErrorKind{}
	for kind := range kindStatuses {
		status := HTTPStatus(kind)
		if previous, ok := seen[status]; ok {
			t.Errorf("%s and %s both map to %d", previous, kind, status)
		}
		seen[status] = kind
		if got := KindForStatus(status); got != kind {
			t.Errorf("KindForStatus(%d) = %s, want %s", status, got, kind)
		}
	}
	if got := HTTPStatus("unknown"); got != http.StatusInternalServerError {
		t.Errorf("HTTPStatus(unknown) = %d, want 500", got)
	}
	if got := KindForStatus(http.StatusTeapot); got != KindInternal {
		t.Errorf("KindForStatus(418) = %s, want internal", got)
	}
}
