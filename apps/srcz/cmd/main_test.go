package cmd

import (
	"fmt"
	"os"
	"testing"
)

// TestMain runs the CLI instead of the tests when SRCZ_CMD_TEST_EXEC=1, so
// tests can execute the test binary as srcz, including detached daemons.
func TestMain(m *testing.M) {
	if os.Getenv("SRCZ_CMD_TEST_EXEC") == "1" {
		if err := Execute(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "  %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
