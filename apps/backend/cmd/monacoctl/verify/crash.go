package verify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func (s *Stack) crash(ctx context.Context, point faultpoint.Name) error {
	w := s.procs[procWorker]
	select {
	case <-w.exited:
	case <-ctx.Done():
		return fmt.Errorf("worker did not crash at %s: %w", point, context.Cause(ctx))
	}
	if want := "faultpoint: crash at " + string(point); !strings.Contains(s.Logs.tail(procWorker, 50), want) {
		return fmt.Errorf("%w: worker exited (%w) without %q\n%s", errFailed, w.err, want, s.Logs.tail(procWorker, 20))
	}
	s.Crashes++
	s.armed = false
	return s.startWorker(ctx)
}

func (s *Stack) arm(ctx context.Context) error {
	if s.armed || s.opts.Faultpoint == "" {
		return nil
	}
	err := s.procs[procWorker].stop(ctx)
	s.armed = true
	return errors.Join(err, s.startWorker(ctx, "MONACO_FAULTPOINT="+s.opts.Faultpoint))
}
