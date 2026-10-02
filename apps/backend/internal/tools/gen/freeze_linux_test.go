//go:build linux

package gen_test

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func freeze(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() { _ = setImmutable(path, 0) })
	if err := setImmutable(path, 0x10); err != nil {
		mode := os.FileMode(0o400)
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			mode = 0o500
		}
		if chmodErr := os.Chmod(path, mode); chmodErr != nil {
			t.Fatal(chmodErr)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
		return
	}
}

func setImmutable(path string, flags int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return unix.IoctlSetPointerInt(int(f.Fd()), unix.FS_IOC_SETFLAGS, flags)
}
