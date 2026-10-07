//go:build unix

package agents

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUnlock_freesTheSlotWhileACopyOfItsDescriptorIsStillOpen(t *testing.T) {
	t.Parallel()
	slot := filepath.Join(t.TempDir(), "0")
	held, ok, err := tryLock(slot)
	if err != nil || !ok {
		t.Fatalf("take the slot: %v %v", ok, err)
	}
	copied, err := syscall.Dup(int(held.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Close(copied) }()
	unlock(held)
	again, ok, err := tryLock(slot)
	if err != nil || !ok {
		t.Fatalf("a forked child's copy of the descriptor kept the released slot locked: %v %v", ok, err)
	}
	unlock(again)
}

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
