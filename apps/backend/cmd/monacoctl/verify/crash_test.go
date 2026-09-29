package verify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestStack_crashRestartsTheWorkerUnarmedAndArmReArmsIt(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "ok")
	o.Faultpoint = string(faultpoint.AfterPublish)
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.arm(t.Context()); err != nil {
		t.Fatalf("arm while armed: %v", err)
	}
	for range 2 {
		if err := s.crash(t.Context(), faultpoint.AfterPublish); err != nil {
			t.Fatalf("crash: %v", err)
		}
		if !s.procs[procWorker].running() || s.armed {
			t.Fatal("worker not restarted unarmed")
		}
		if err := s.arm(t.Context()); err != nil || !s.armed {
			t.Fatalf("arm: %v", err)
		}
	}
	if s.Crashes != 2 {
		t.Fatalf("Crashes = %d, want 2", s.Crashes)
	}
	checkCrashFailures(t, s)
}

func checkCrashFailures(t *testing.T, s *Stack) {
	t.Helper()
	if err := s.crash(t.Context(), faultpoint.AfterSign); err == nil ||
		!strings.Contains(err.Error(), `without "faultpoint: crash at after-sign"`) {
		t.Fatalf("crash at the wrong point = %v", err)
	}
	s.opts.Faultpoint, s.armed = "", false
	if err := s.arm(t.Context()); err != nil {
		t.Fatalf("arm without a faultpoint: %v", err)
	}
	if err := s.startWorker(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := s.crash(ctx, faultpoint.AfterPublish); err == nil ||
		!strings.Contains(err.Error(), "worker did not crash at after-publish") {
		t.Fatalf("crash of a worker that never crashes = %v", err)
	}
}
