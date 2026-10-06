package verify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
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
	if err := s.awaitEarlierEventsPublished(ctx); err != nil {
		return err
	}
	err := s.procs[name].stop(ctx)
	s.armed = true
	s.armedName = name
	return errors.Join(err, s.startProcess(ctx, name, "MONACO_FAULTPOINT="+fault))
}

func (s *Stack) awaitEarlierEventsPublished(ctx context.Context) error {
	tick := time.NewTicker(s.pollInterval())
	defer tick.Stop()
	for {
		probe, cancel := detached(ctx)
		busy, err := s.busy(probe)
		cancel()
		if err != nil || busy == "" {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s before arming: %w", busy, context.Cause(ctx))
		case <-tick.C:
		}
	}
}

func (s *Stack) busy(ctx context.Context) (string, error) {
	busy := ""
	stream, err := s.NATS.JS.Stream(ctx, bus.StreamEvents)
	if err == nil {
		consumers := stream.ListConsumers(ctx)
		for info := range consumers.Info() {
			busy = cmp.Or(busy, consumerBusy(info))
		}
		err = consumers.Err()
	}
	if err == nil && busy == "" {
		var left int
		err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE published_at IS NULL`).Scan(&left)
		busy = unpublished(left)
	}
	if err != nil {
		return "", fmt.Errorf("read the event backlog: %w", err)
	}
	return busy, nil
}

func consumerBusy(info *jetstream.ConsumerInfo) string {
	if info.NumPending == 0 && info.NumAckPending == 0 {
		return ""
	}
	return fmt.Sprintf("consumer %s has %d pending and %d unacked messages",
		info.Name, info.NumPending, info.NumAckPending)
}

func unpublished(left int) string {
	if left == 0 {
		return ""
	}
	return fmt.Sprintf("%d events unpublished", left)
}

func (s *Stack) startProcess(ctx context.Context, name string, extra ...string) error {
	if name == procAPI {
		return s.startAPI(ctx, extra...)
	}
	return s.startWorker(ctx, extra...)
}

func processFor(u Unit, point faultpoint.Name) string {
	if slices.Contains(
		[]faultpoint.Name{faultpoint.AfterPublish, faultpoint.AfterCreate, faultpoint.AfterExecute},
		point,
	) {
		return procWorker
	}
	kind, _ := u.Flow.TriggerKind(u.Command)
	if kind == tools.TriggerRoute {
		return procAPI
	}
	return procWorker
}
