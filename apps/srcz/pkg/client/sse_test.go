package client

import (
	"bufio"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/srcz/pkg/contract"
)

func TestServerSentEventsParse(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(": srcz\n\nevent: walkthrough\ndata: {\"project\":\"/p\",\"id\":\"tour\",\"title\":\"Tour\",\"created\":true}\n\n: heartbeat\n\nevent: other\ndata: {}\n\n"))
	name, data, err := readEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	event, ok, err := daemonEvent(name, data)
	if err != nil || !ok || !reflect.DeepEqual(event.Walkthrough, &contract.WalkthroughEvent{Project: "/p", ID: "tour", Title: "Tour", Created: true}) {
		t.Fatalf("event = %+v, %v, %v", event, ok, err)
	}
	name, data, err = readEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := daemonEvent(name, data); ok || err != nil {
		t.Errorf("unknown event = %v, %v; want skipped", ok, err)
	}
	if _, _, err := readEvent(reader); !errors.Is(err, io.EOF) {
		t.Errorf("end = %v, want io.EOF", err)
	}
}
