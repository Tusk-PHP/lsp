//go:build wasip1

package diagnostics

import "os/exec"

const startsProcesses = false

func stopProcessTreeOnCancel(*exec.Cmd) {}
