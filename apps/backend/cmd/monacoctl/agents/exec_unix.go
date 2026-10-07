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
	return classifyKillErr(syscall.Kill(-process.Pid, syscall.SIGKILL))
}

func classifyKillErr(err error) error {
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	if err != nil {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

func tryLock(name string) (*os.File, bool, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", name, err)
	}
	if locked := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil; !locked {
		_ = f.Close()
		return nil, false, nil
	}
	return f, true, nil
}

func unlock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
