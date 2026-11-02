//go:build !darwin && !linux

package daemon

import "os"

// Listen is unsupported on this host.
func Listen(string) (*Listener, error) { return nil, ErrUnsupported }

// OpenLog is unsupported on this host.
func OpenLog(string) (*os.File, error) { return nil, ErrUnsupported }
