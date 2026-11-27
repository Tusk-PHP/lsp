//go:build !unix && !windows

package diagnostics

import "os/exec"

func stopProcessTreeOnCancel(*exec.Cmd) {}
