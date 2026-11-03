package lsp

import (
	"bufio"
	"context"
	"errors"
	"os"
	"testing"
)

func TestClosedStdoutPreservesProcessFailure(t *testing.T) {
	stdout, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	// exec.Cmd.Wait closes StdoutPipe. A read scheduled afterward must
	// leave the failure to the process waiter, which has the stderr tail.
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	c := &conn{done: make(chan struct{})}
	c.read(bufio.NewReader(stdout))
	select {
	case <-c.done:
		t.Fatalf("stdout close published failure before process exit: %v", c.err)
	default:
	}
	failure := errors.New("exit status 3: fake server crashed")
	c.close(failure)
	if err := c.call(context.Background(), "textDocument/definition", nil, nil); !errors.Is(err, failure) {
		t.Fatalf("call error = %v, want process failure %v", err, failure)
	}
}
