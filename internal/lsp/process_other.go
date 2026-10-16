//go:build !unix

package lsp

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
func killProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
