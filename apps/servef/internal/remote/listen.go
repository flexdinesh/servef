package remote

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// The ports tried without an explicit one: the CLI server's range.
const (
	defaultPortStart = 7971
	defaultPortEnd   = 7980
)

// ErrPortBusy reports that the port, or every default port, is taken.
var ErrPortBusy = errors.New("port is busy")

// Listen binds TCP on host, or every interface when host is empty: an explicit
// port when port is set; otherwise preferred, then the default range.
func Listen(host string, port *int, preferred int) (net.Listener, error) {
	listen := func(candidate int) (net.Listener, error) {
		return net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(candidate)))
	}
	if port != nil {
		listener, err := listen(*port)
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("%w: %d", ErrPortBusy, *port)
		}
		return listener, err
	}
	candidates := make([]int, 0, defaultPortEnd-defaultPortStart+2)
	if preferred > 0 {
		candidates = append(candidates, preferred)
	}
	for candidate := defaultPortStart; candidate <= defaultPortEnd; candidate++ {
		if candidate != preferred {
			candidates = append(candidates, candidate)
		}
	}
	for _, candidate := range candidates {
		listener, err := listen(candidate)
		if err == nil {
			return listener, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %d to %d", ErrPortBusy, defaultPortStart, defaultPortEnd)
}

// Port is a TCP listener's port.
func Port(listener net.Listener) (int, error) {
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listener address %s is not TCP", listener.Addr())
	}
	return address.Port, nil
}
