//go:build unix

package agents

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func configureExec(cmd *exec.Cmd) {
	cmd.WaitDelay = execWaitDelay()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process) }
}

func killGroup(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	if err != nil {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}
