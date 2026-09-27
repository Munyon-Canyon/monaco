package testkit

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func CrashAt(tb testing.TB, name faultpoint.Name, run func(ctx context.Context) error) {
	tb.Helper()
	if !faultpoint.Enabled {
		tb.Fatal("testkit.CrashAt: built without -tags faultpoints")
	}
	var err error
	crashed := func() (p any) {
		defer func() { p = recover() }()
		err = run(faultpoint.Armed(tb.Context(), name))
		return nil
	}()
	switch {
	case crashed == nil:
		tb.Fatalf("testkit.CrashAt: the run returned (err %v) without reaching %s", err, name)
	case crashed != faultpoint.Crash{Name: name}:
		panic(crashed)
	}
	if err := run(tb.Context()); err != nil {
		tb.Fatalf("testkit.CrashAt: the restarted run after %s: %v", name, err)
	}
}
