package testkit_test

import (
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fakeWallet struct {
	testkit.Faults
	sent int
}

func (w *fakeWallet) Send() error {
	if err := w.Check("Send"); err != nil {
		return err
	}
	w.sent++
	return nil
}

func TestFaultsZeroValuePasses(t *testing.T) {
	t.Parallel()
	var w fakeWallet
	if err := w.Send(); err != nil || w.sent != 1 {
		t.Fatalf("Send() = %v, sent %d", err, w.sent)
	}
}

func TestFaultsFailAppliesToEveryCallOfThatOp(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeInternal, "fake.Send")
	var w fakeWallet
	w.Fail("Send", down)
	w.Fail("Other", errs.New(errs.CodeInvalidInput, "fake.Other"))
	for range 3 {
		if err := w.Send(); !errors.Is(err, down) {
			t.Fatalf("Send() = %v, want %v", err, down)
		}
	}
	if w.sent != 0 {
		t.Fatalf("a failed Send still sent %d", w.sent)
	}
	w.Fail("Send", nil)
	if err := w.Send(); err != nil {
		t.Fatalf("Send() after Fail(nil) = %v", err)
	}
}

func TestFaultsFailOnceFailsTheNextCallsInOrder(t *testing.T) {
	t.Parallel()
	first := errs.New(errs.CodeInternal, "fake.first")
	second := errs.New(errs.CodeInvalidInput, "fake.second")
	var w fakeWallet
	w.FailOnce("Send", first)
	w.FailOnce("Send", second)
	for i, want := range []error{first, second, nil} {
		if err := w.Send(); !errors.Is(err, want) || (want == nil && err != nil) {
			t.Fatalf("call %d Send() = %v, want %v", i, err, want)
		}
	}
	if w.sent != 1 {
		t.Fatalf("sent %d, want 1", w.sent)
	}
}

func TestFaultsFailOnceRunsBeforeFail(t *testing.T) {
	t.Parallel()
	always := errs.New(errs.CodeInternal, "fake.always")
	once := errs.New(errs.CodeInvalidInput, "fake.once")
	var w fakeWallet
	w.Fail("Send", always)
	w.FailOnce("Send", once)
	if err := w.Send(); !errors.Is(err, once) {
		t.Fatalf("first Send() = %v, want the once error", err)
	}
	if err := w.Send(); !errors.Is(err, always) {
		t.Fatalf("second Send() = %v, want the standing error", err)
	}
}
