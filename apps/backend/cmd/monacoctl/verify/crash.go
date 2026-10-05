package verify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func (s *Stack) crash(ctx context.Context, point faultpoint.Name) error {
	return s.crashProcess(ctx, procWorker, point)
}

func (s *Stack) crashUnit(ctx context.Context, u Unit, point faultpoint.Name) error {
	return s.crashProcess(ctx, processFor(u, point), point)
}

func (s *Stack) crashProcess(ctx context.Context, name string, point faultpoint.Name) error {
	p := s.procs[name]
	select {
	case <-p.exited:
	case <-ctx.Done():
		return fmt.Errorf("%s did not crash at %s: %w", name, point, context.Cause(ctx))
	default:
		if name == procAPI {
			if err := p.stop(ctx); err != nil {
				return err
			}
		} else {
			select {
			case <-p.exited:
			case <-ctx.Done():
				return fmt.Errorf("%s did not crash at %s: %w", name, point, context.Cause(ctx))
			}
		}
	}
	want := "faultpoint: crash at " + string(point)
	if name == procAPI {
		s.Crashes++
		s.armed = false
		s.armedName = ""
		return s.startProcess(ctx, name)
	}
	if !strings.Contains(s.Logs.tail(name, 50), want) && !strings.Contains(s.Logs.lastPanicOrLine(name), want) {
		return fmt.Errorf(
			"%w: %s exited (%w) without %q\n%s", errFailed, name, p.err, want, s.Logs.tail(name, 20),
		)
	}
	s.Crashes++
	s.armed = false
	s.armedName = ""
	return s.startProcess(ctx, name)
}

func (s *Stack) arm(ctx context.Context) error {
	if s.armed || s.opts.Faultpoint == "" {
		return nil
	}
	return s.armProcess(ctx, procWorker, s.opts.Faultpoint)
}

func (s *Stack) armUnit(ctx context.Context, u Unit) error {
	if s.opts.Faultpoint == "" {
		return nil
	}
	return s.armProcess(ctx, processFor(u, faultpoint.Name(s.opts.Faultpoint)), s.opts.Faultpoint+"@"+u.Flow.ID)
}

func (s *Stack) armProcess(ctx context.Context, name, fault string) error {
	if s.armed {
		return nil
	}
	err := s.procs[name].stop(ctx)
	s.armed = true
	s.armedName = name
	return errors.Join(err, s.startProcess(ctx, name, "MONACO_FAULTPOINT="+fault))
}

func (s *Stack) startProcess(ctx context.Context, name string, extra ...string) error {
	if name == procAPI {
		return s.startAPI(ctx, extra...)
	}
	return s.startWorker(ctx, extra...)
}

func processFor(u Unit, point faultpoint.Name) string {
	if point == faultpoint.AfterPublish {
		return procWorker
	}
	kind, _ := u.Flow.TriggerKind(u.Command)
	if kind == tools.TriggerRoute {
		return procAPI
	}
	return procWorker
}
