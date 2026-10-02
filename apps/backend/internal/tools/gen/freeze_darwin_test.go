//go:build darwin

package gen_test

import (
	"syscall"
	"testing"
)

func freeze(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Chflags(path, 0x2); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Chflags(path, 0) })
}

func thaw(path string) error { return syscall.Chflags(path, 0) }
