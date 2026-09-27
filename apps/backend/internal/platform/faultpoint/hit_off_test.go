//go:build !faultpoints

package faultpoint_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestEnabled_falseWithoutTheTag(t *testing.T) {
	t.Parallel()
	if faultpoint.Enabled {
		t.Fatal("Enabled = true in a build without -tags faultpoints")
	}
}

func TestArmed_returnsTheContextUnchangedWithoutTheTag(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	armed := faultpoint.Armed(ctx, faultpoint.BeforeCommit)
	if armed != ctx || faultpoint.ArmedAfter(ctx, faultpoint.AfterSign, 1) != ctx {
		t.Fatal("Armed wrapped the context in a build without -tags faultpoints")
	}
	if p := recovered(func() { faultpoint.Hit(armed, faultpoint.BeforeCommit) }); p != nil {
		t.Fatalf("Hit panicked with %v in a build without -tags faultpoints", p)
	}
}

func TestConfigure_refusesARegisteredNameWithoutTheTag(t *testing.T) {
	t.Parallel()
	got := reason(t, faultpoint.Configure(string(faultpoint.BeforeCommit)))
	if got != "built without -tags faultpoints" {
		t.Fatalf("reason = %q, want built without -tags faultpoints", got)
	}
}
