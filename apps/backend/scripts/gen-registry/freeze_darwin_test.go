//go:build darwin

package main

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
