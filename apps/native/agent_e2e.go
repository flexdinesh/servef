//go:build e2e

package main

// End-to-end builds (go build -tags e2e) link mygo's automation plugin:
// with MYGO_AUTOMATION set, the built app serves the automation protocol
// that mygo-agent drives (decision 0086). Release builds never carry it.
import _ "github.com/egoist/mygo/plugins/automation"
