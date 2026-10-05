package faultpoint_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func recovered(fn func()) (p any) {
	defer func() { p = recover() }()
	fn()
	return nil
}

func reason(t *testing.T, err error) string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalidInput || e.Op != "faultpoint.Configure" {
		t.Fatalf("Configure error = %v, want invalid_input from faultpoint.Configure", err)
	}
	for _, a := range e.Attrs {
		if a.Key == "reason" {
			return a.Value.String()
		}
	}
	t.Fatalf("Configure error %v carries no reason", err)
	return ""
}

func TestNames_listsTheRegisteredPointsSorted(t *testing.T) {
	t.Parallel()
	want := []faultpoint.Name{
		faultpoint.AfterBroadcast, faultpoint.AfterCreate, faultpoint.AfterExecute, faultpoint.AfterPublish,
		faultpoint.AfterSellConfirm, faultpoint.AfterSellRequest, faultpoint.AfterSign, faultpoint.BeforeCommit,
	}
	got := faultpoint.Names()
	if !slices.Equal(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if faultpoint.Names()[0] != faultpoint.AfterBroadcast {
		t.Fatal("Names shares its backing array with callers")
	}
}

func TestKnown_acceptsOnlyRegisteredNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"after-create", true},
		{"after-sign", true},
		{"after-execute", true},
		{"before-commit", true},
		{"after-publish", true},
		{"", false},
		{"Before-Commit", false},
		{"before-commit ", false},
		{"after-everything", false},
	} {
		if got := faultpoint.Known(tc.name); got != tc.want {
			t.Errorf("Known(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCrash_errorNamesThePoint(t *testing.T) {
	t.Parallel()
	var err error = faultpoint.Crash{Name: faultpoint.BeforeCommit}
	if err.Error() != "faultpoint: crash at before-commit" {
		t.Fatalf("Error = %q", err.Error())
	}
}

func TestIsCrash_trueOnlyForACrash(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		p    any
		want bool
	}{
		{"crash", faultpoint.Crash{Name: faultpoint.AfterPublish}, true},
		{"nil", nil, false},
		{"string panic", "boom", false},
		{"error panic", errs.New(errs.CodePanic, "x"), false},
		{"crash pointer", &faultpoint.Crash{Name: faultpoint.AfterPublish}, false},
	} {
		if got := faultpoint.IsCrash(tc.p); got != tc.want {
			t.Errorf("IsCrash(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestConfigure_emptyNameArmsNothing(t *testing.T) {
	t.Parallel()
	if err := faultpoint.Configure(""); err != nil {
		t.Fatalf("Configure(\"\") = %v, want nil", err)
	}
	if p := recovered(func() { faultpoint.Hit(t.Context(), faultpoint.BeforeCommit) }); p != nil {
		t.Fatalf("Hit after Configure(\"\") panicked with %v", p)
	}
}

func TestConfigure_rejectsAnUnknownName(t *testing.T) {
	t.Parallel()
	if got := reason(t, faultpoint.Configure("after-everything")); got != "unknown faultpoint" {
		t.Fatalf("reason = %q, want unknown faultpoint", got)
	}
}

func TestConfigure_rejectsAnInvalidScopedName(t *testing.T) {
	t.Parallel()
	if got := reason(t, faultpoint.Configure("before-commit@")); got != "invalid faultpoint" {
		t.Fatalf("reason = %q, want invalid faultpoint", got)
	}
}

func TestHit_unarmedNeverCrashes(t *testing.T) {
	t.Parallel()
	ctx := faultpoint.Armed(t.Context(), faultpoint.AfterSign)
	p := recovered(func() {
		faultpoint.Hit(t.Context(), faultpoint.AfterCreate)
		faultpoint.Hit(t.Context(), faultpoint.AfterSign)
		faultpoint.Hit(ctx, faultpoint.AfterExecute)
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		faultpoint.Hit(ctx, faultpoint.AfterPublish)
	})
	if p != nil {
		t.Fatalf("Hit panicked with %v at a point nothing armed", p)
	}
}
