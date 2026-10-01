//go:build !unix

package agents

import (
	"os"
	"os/exec"
)

func configureExec(cmd *exec.Cmd) {
	cmd.WaitDelay = execWaitDelay()
}

func killGroup(*os.Process) error { return nil }
