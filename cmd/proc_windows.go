//go:build windows

package cmd

import "os/exec"

func setDetach(cmd *exec.Cmd) {
	// Windows doesn't have Setsid; process detach handled differently
}
