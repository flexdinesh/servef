package main

import (
	"fmt"
	"os"

	"github.com/flexdinesh/srcz/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  %v\n", err)
		os.Exit(1)
	}
}
