//go:build windows

package diagnostics

import (
	"errors"
	"os/exec"
	"strconv"
)

const startsProcesses = true

func stopProcessTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return errors.Join(err, cmd.Process.Kill())
		}
		return nil
	}
}
