//go:build unix

package agents

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestKillGroup_nilProcessReportsAlreadyDone(t *testing.T) {
	t.Parallel()
	if err := killGroup(nil); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("killGroup(nil) = %v, want os.ErrProcessDone", err)
	}
}

func TestClassifyKillErr_wrapsASignalFailureOtherThanESRCH(t *testing.T) {
	t.Parallel()
	err := classifyKillErr(syscall.EPERM)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("classifyKillErr(EPERM) = %v, want a wrapped EPERM", err)
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatalf("classifyKillErr(EPERM) = %v, want it to wrap syscall.EPERM", err)
	}
}
