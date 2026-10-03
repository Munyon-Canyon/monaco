//go:build faultpoints

package faultpoint_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestEnabled_trueUnderTheTag(t *testing.T) {
	t.Parallel()
	if !faultpoint.Enabled {
		t.Fatal("Enabled = false in a -tags faultpoints build")
	}
}

func TestHit_armedContextCrashesOnlyThatContextAtThatPoint(t *testing.T) {
	t.Parallel()
	armed := faultpoint.Armed(t.Context(), faultpoint.BeforeCommit)
	if p := recovered(func() { faultpoint.Hit(armed, faultpoint.AfterPublish) }); p != nil {
		t.Fatalf("Hit at another point panicked with %v", p)
	}
	if p := recovered(func() { faultpoint.Hit(t.Context(), faultpoint.BeforeCommit) }); p != nil {
		t.Fatalf("Hit on an unarmed context panicked with %v", p)
	}
	p := recovered(func() { faultpoint.Hit(armed, faultpoint.BeforeCommit) })
	if p != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("Hit on the armed point recovered %v, want Crash{before-commit}", p)
	}
}

func TestArmedAfter_crashesFromTheHitAfterTheSkippedOnes(t *testing.T) {
	t.Parallel()
	ctx := faultpoint.ArmedAfter(t.Context(), faultpoint.AfterSign, 2)
	got := make([]any, 0, 4)
	for range 4 {
		faultpoint.Hit(ctx, faultpoint.AfterCreate)
		got = append(got, recovered(func() { faultpoint.Hit(ctx, faultpoint.AfterSign) }))
	}
	crash := faultpoint.Crash{Name: faultpoint.AfterSign}
	if got[0] != nil || got[1] != nil || got[2] != crash || got[3] != crash {
		t.Fatalf("hits 1..4 recovered %v, want nil, nil, then Crash{after-sign} from the third on", got)
	}
}

func TestConfigure_armsEveryContextInTheProcessUntilCleared(t *testing.T) {
	t.Cleanup(func() { _ = faultpoint.Configure("") })
	if err := faultpoint.Configure(string(faultpoint.AfterSign)); err != nil {
		t.Fatal(err)
	}
	p := recovered(func() { faultpoint.Hit(t.Context(), faultpoint.AfterSign) })
	if p != (faultpoint.Crash{Name: faultpoint.AfterSign}) {
		t.Fatalf("Hit after Configure recovered %v, want Crash{after-sign}", p)
	}
	if p := recovered(func() { faultpoint.Hit(t.Context(), faultpoint.AfterCreate) }); p != nil {
		t.Fatalf("Hit at another point panicked with %v", p)
	}
	if err := faultpoint.Configure(""); err != nil {
		t.Fatal(err)
	}
	if p := recovered(func() { faultpoint.Hit(t.Context(), faultpoint.AfterSign) }); p != nil {
		t.Fatalf("Hit after Configure(\"\") panicked with %v", p)
	}
}

func TestConfigure_scopesTheProcessArmToAFlow(t *testing.T) {
	t.Cleanup(func() { _ = faultpoint.Configure("") })
	if err := faultpoint.Configure("before-commit@01"); err != nil {
		t.Fatal(err)
	}
	if got := faultpoint.ConfiguredFlow(); got != "01" {
		t.Fatalf("ConfiguredFlow = %q, want 01", got)
	}
	wrongFlow := faultpoint.WithFlow(t.Context(), "00")
	if p := recovered(func() { faultpoint.Hit(wrongFlow, faultpoint.BeforeCommit) }); p != nil {
		t.Fatalf("Hit in flow 00 recovered %v", p)
	}
	rightFlow := faultpoint.WithFlow(t.Context(), "01")
	p := recovered(func() { faultpoint.Hit(rightFlow, faultpoint.BeforeCommit) })
	if p != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("Hit in flow 01 recovered %v, want Crash{before-commit}", p)
	}
	if err := faultpoint.Configure(""); err != nil {
		t.Fatal(err)
	}
	if got := faultpoint.ConfiguredFlow(); got != "" {
		t.Fatalf("ConfiguredFlow after clear = %q, want empty", got)
	}
}
