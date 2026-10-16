package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// maxMessage bounds one message from a server.
const maxMessage = 64 << 20

// errClosed reports a connection whose server exited.
var errClosed = errors.New("language server exited")

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("language server error %d: %s", e.Code, e.Message)
}

// incoming is any message a server sends: a response, a request, or a
// notification.
type incoming struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

// conn speaks JSON-RPC 2.0 with Content-Length framing over a server's
// stdio. Calls are concurrent; responses are matched by ID.
type conn struct {
	out     io.Writer
	writeMu sync.Mutex

	mu      sync.Mutex
	next    int64
	pending map[int64]chan incoming
	err     error
	done    chan struct{}

	// answer replies to requests the server sends the client.
	answer func(method string, params json.RawMessage) (any, *rpcError)
}

func newConn(in io.Reader, out io.Writer, answer func(string, json.RawMessage) (any, *rpcError)) *conn {
	c := &conn{out: out, pending: make(map[int64]chan incoming), done: make(chan struct{}), answer: answer}
	go c.read(bufio.NewReader(in))
	return c
}

func (c *conn) read(in *bufio.Reader) {
	for {
		message, err := readMessage(in)
		if err != nil {
			// At EOF the server is exiting; whoever waits on the process
			// closes with its exit status and stderr.
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				c.close(err)
			}
			return
		}
		switch {
		case message.Method != "" && len(message.ID) > 0:
			result, failure := c.answer(message.Method, message.Params)
			_ = c.write(reply{JSONRPC: "2.0", ID: message.ID, Result: result, Error: failure})
		case message.Method == "" && len(message.ID) > 0:
			id, err := strconv.ParseInt(string(message.ID), 10, 64)
			if err != nil {
				continue
			}
			c.mu.Lock()
			waiting := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if waiting != nil {
				waiting <- message
			}
		}
		// Notifications (diagnostics, progress, logs) are not used.
	}
}

func readMessage(in *bufio.Reader) (incoming, error) {
	header, err := textproto.NewReader(in).ReadMIMEHeader()
	if err != nil {
		return incoming{}, err
	}
	length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil || length < 0 || length > maxMessage {
		return incoming{}, fmt.Errorf("invalid Content-Length %q", header.Get("Content-Length"))
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(in, body); err != nil {
		return incoming{}, err
	}
	var message incoming
	if err := json.Unmarshal(body, &message); err != nil {
		return incoming{}, err
	}
	return message, nil
}

func (c *conn) write(message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.out.Write(body)
	return err
}

func (c *conn) close(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	if err == nil || errors.Is(err, io.EOF) {
		err = errClosed
	}
	c.err = err
	close(c.done)
}

func (c *conn) notify(method string, params any) error {
	return c.write(notification{JSONRPC: "2.0", Method: method, Params: params})
}

// call sends a request and decodes its result into out unless out is nil.
// A cancelled context cancels the request on the server too.
func (c *conn) call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	c.next++
	id := c.next
	waiting := make(chan incoming, 1)
	c.pending[id] = waiting
	c.mu.Unlock()

	if err := c.write(request{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.forget(id)
		return err
	}
	select {
	case message := <-waiting:
		if message.Error != nil {
			return message.Error
		}
		if out == nil || len(message.Result) == 0 {
			return nil
		}
		return json.Unmarshal(message.Result, out)
	case <-ctx.Done():
		c.forget(id)
		_ = c.notify("$/cancelRequest", map[string]int64{"id": id})
		return ctx.Err()
	case <-c.done:
		return c.err
	}
}

func (c *conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}
