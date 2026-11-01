//go:build !darwin && !linux

package daemon

// DefaultSocket is the per-user socket path (decision 0061).
func DefaultSocket() (string, error) { return "", ErrUnsupported }
