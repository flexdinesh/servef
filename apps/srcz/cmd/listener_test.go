package cmd

import (
	"bytes"
	"errors"
	"net"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestOpenListenerTriesDefaultPortsInOrder(t *testing.T) {
	disableColor(t)
	var attempts []int
	var output bytes.Buffer
	listener, err := openListener(defaultHost, options{}, &output, func(_ string, port int) (net.Listener, error) {
		attempts = append(attempts, port)
		if port < 7973 {
			return nil, syscall.EADDRINUSE
		}
		return net.Listen("tcp", "127.0.0.1:0")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if want := []int{7971, 7972, 7973}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	if want := "  port 7971 is busy; trying 7972.\n  port 7972 is busy; trying 7973.\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestOpenListenerReportsExhaustedDefaultRange(t *testing.T) {
	disableColor(t)
	var attempts []int
	var output bytes.Buffer
	_, err := openListener(defaultHost, options{}, &output, func(_ string, port int) (net.Listener, error) {
		attempts = append(attempts, port)
		return nil, syscall.EADDRINUSE
	})
	if !errors.Is(err, errDefaultPortsBusy) {
		t.Fatalf("error = %v, want %v", err, errDefaultPortsBusy)
	}
	if len(attempts) != 10 || attempts[0] != 7971 || attempts[len(attempts)-1] != 7980 {
		t.Fatalf("attempts = %v", attempts)
	}
	if got := strings.Count(output.String(), "is busy"); got != 10 {
		t.Fatalf("busy messages = %d, want 10", got)
	}
	if !strings.HasSuffix(output.String(), "  port 7980 is busy.\n") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestOpenListenerUsesExplicitPortOnly(t *testing.T) {
	var attempts []int
	listener, err := openListener(defaultHost, options{port: 8123, portSet: true}, &bytes.Buffer{}, func(_ string, port int) (net.Listener, error) {
		attempts = append(attempts, port)
		return net.Listen("tcp", "127.0.0.1:0")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if want := []int{8123}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
}

func TestConfirmStopServers(t *testing.T) {
	tests := []struct {
		answer string
		want   bool
	}{
		{answer: "yes\n", want: true},
		{answer: "Y\n", want: true},
		{answer: "no\n", want: false},
		{answer: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.answer, func(t *testing.T) {
			disableColor(t)
			var output bytes.Buffer
			if got := confirmStopServers(strings.NewReader(test.answer), &output, 10); got != test.want {
				t.Fatalf("confirmStopServers() = %t, want %t", got, test.want)
			}
			if !strings.HasPrefix(output.String(), "  all 10 default ports are busy.") {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}
