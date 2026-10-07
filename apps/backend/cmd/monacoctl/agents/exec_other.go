//go:build !unix

package agents

import (
	"fmt"
	"os"
	"os/exec"
)

func configureExec(cmd *exec.Cmd) {
	cmd.WaitDelay = execWaitDelay()
}

func killGroup(*os.Process) error { return nil }

func tryLock(name string) (*os.File, bool, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", name, err)
	}
	return f, true, nil
}

func unlock(f *os.File) { _ = f.Close() }
