package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// An answer outside the contract says what the server sent, so a client can
// tell it from a server it could not reach.
func TestAnswersOutsideTheContractCarryTheirStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"error page", http.StatusBadGateway, "<html>Bad gateway</html>"},
		{"no error text", http.StatusInternalServerError, "{}"},
		{"malformed success", http.StatusOK, "not json"},
		{"wrong shape", http.StatusOK, `{"formatted":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := client.NewTCP(server.URL).Format(context.Background(), contract.FormatRequest{Project: "/p", Path: "a.json"})
			var response *client.ResponseError
			if !errors.As(err, &response) || response.Status != tc.status {
				t.Fatalf("error %T %v, want status %d", err, err, tc.status)
			}
		})
	}
}

func TestContractErrorsAndDeadServersAreNotResponseErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"bad json"}`))
	}))
	c := client.NewTCP(server.URL)
	_, err := c.Format(context.Background(), contract.FormatRequest{Project: "/p", Path: "a.json"})
	var failed *client.Error
	if !errors.As(err, &failed) || failed.Message != "bad json" {
		t.Fatalf("error %T %v", err, err)
	}
	server.Close()
	_, err = c.Format(context.Background(), contract.FormatRequest{Project: "/p", Path: "a.json"})
	var response *client.ResponseError
	if err == nil || errors.As(err, &response) {
		t.Fatalf("a dead server: %T %v", err, err)
	}
}
