package bus_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func TestDispatch_transientPostgresErrorsWrappedAsInternalNakOutsideTheUnitOfWork(t *testing.T) {
	t.Parallel()
	states := map[string]string{
		"too many connections": "53300",
		"statement timeout":    "57014",
		"lock timeout":         "55P03",
		"failover read only":   "25006",
		"admin shutdown":       "57P01",
		"connection exception": "08006",
	}
	for name, sqlState := range states {
		blip := errs.Wrap(&pgconn.PgError{Code: sqlState}, errs.CodeInternal, "app.read")
		for kind, handler := range map[string]bus.HandlerSpec{
			"own handler": bus.HandleOwn("trading.engine",
				func(context.Context, bus.Delivery, events.SystemPinged) error { return blip }),
			"fetch step": bus.HandleFetched("notify.push",
				func(context.Context, events.SystemPinged) (struct{}, error) { return struct{}{}, blip },
				func(context.Context, db.Tx, events.SystemPinged, struct{}, time.Time) error { return nil }),
		} {
			t.Run(name+" in "+kind, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t)
				reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
				_, msg := h.pingMsg(t)

				reg.Dispatch(h.ctx(t), durable, msg)

				if msg.verdict != "nak" || len(h.deliveries(t)) != 0 {
					t.Fatalf("verdict = %q, deliveries = %v; want nak and no row for SQLSTATE %s",
						msg.verdict, h.deliveries(t), sqlState)
				}
				h.assertLines(t, "bus.dispatched",
					map[string]any{"outcome": "nak", "code": "db_unavailable", "level": "WARN"})
			})
		}
	}
}

func TestDispatch_anIntegrityViolationWrappedAsInternalNaksSoAReorderedEventIsRetried(t *testing.T) {
	t.Parallel()
	for name, sqlState := range map[string]string{
		"check violation":       "23514",
		"foreign key violation": "23503",
		"not null violation":    "23502",
		"unique violation":      "23505",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			handler := bus.HandleOwn("trading.engine", func(context.Context, bus.Delivery, events.SystemPinged) error {
				return errs.Wrap(&pgconn.PgError{Code: sqlState}, errs.CodeInternal, "app.read")
			})
			reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
			_, msg := h.pingMsg(t)

			first := fromMsg(msg, 1)
			reg.Dispatch(h.ctx(t), durable, first)

			if first.verdict != "nak" || first.delay != time.Second {
				t.Fatalf(
					"verdict = %q after %s, want nak after 1s for SQLSTATE %s",
					first.verdict,
					first.delay,
					sqlState,
				)
			}
			if letters := h.deadLetters(t); len(letters) != 0 {
				t.Fatalf("dead letters = %+v, want none while retries remain", letters)
			}
		})
	}
}

func TestDispatch_aPermanentPostgresErrorWrappedAsInternalStillTerms(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := bus.HandleOwn("trading.engine", func(context.Context, bus.Delivery, events.SystemPinged) error {
		return errs.Wrap(&pgconn.PgError{Code: "22003"}, errs.CodeInternal, "app.read")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
	_, msg := h.pingMsg(t)

	reg.Dispatch(h.ctx(t), durable, msg)

	if msg.verdict != "term" {
		t.Fatalf("verdict = %q, want term for a numeric overflow", msg.verdict)
	}
}

func TestDispatch_theFinalDeliveryOfAnIntegrityViolationTermsWithAnAlertLetterThatCarriesThePayload(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler := bus.HandleOwn("treasury.trades", func(context.Context, bus.Delivery, events.SystemPinged) error {
		return errs.Wrap(&pgconn.PgError{Code: "23514", Message: "cabal_positions_units_check"},
			errs.CodeInternal, "app.read")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
	_, msg := h.pingMsg(t)
	last := fromMsg(msg, bus.MaxDeliver)

	reg.Dispatch(h.ctx(t), durable, last)

	if last.verdict != "term" {
		t.Fatalf("verdict = %q on delivery %d, want term", last.verdict, bus.MaxDeliver)
	}
	letters := h.deadLetters(t)
	if len(letters) != 1 {
		t.Fatalf("dead letters = %+v, want exactly one", letters)
	}
	got := letters[0]
	if got.Handler != "treasury.trades" || got.Consumer != durable || !got.Alert || got.Advisory != nil ||
		!bytes.Equal(got.Data, compact(t, msg.Data())) || !strings.Contains(got.Error, "cabal_positions_units_check") {
		t.Fatalf("letter = %+v, want the handler, an alert mark, the payload %s and the constraint", got, msg.Data())
	}
	h.assertLines(t, "bus.dispatched", map[string]any{"outcome": "term", "code": "internal", "level": "ERROR"})
}
