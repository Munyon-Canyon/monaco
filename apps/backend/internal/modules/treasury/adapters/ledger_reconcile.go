package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	LedgerReconcileGrace    = 10 * time.Minute
	ledgerReconcileLookback = 7 * 24 * time.Hour
	ledgerReconcileRows     = 100
	ledgerReconcileAlerts   = 20
)

type LedgerReconcile struct {
	Reads sqlc.DBTX
	Clock clock.Clock
	Every time.Duration
}

func (LedgerReconcile) Name() string { return "treasury.ledger-reconcile" }

func (p LedgerReconcile) Interval() time.Duration { return p.Every }

type unbooked struct {
	kind    string
	id      string
	cabalID uuid.UUID
	since   time.Time
}

func (p LedgerReconcile) Tick(ctx context.Context) (poller.Report, error) {
	const op = "treasury.LedgerReconcile.Tick"
	now := p.Clock.Now()
	window := struct{ after, before time.Time }{now.Add(-ledgerReconcileLookback), now.Add(-LedgerReconcileGrace)}
	q := sqlc.New(p.Reads)
	swaps, err := q.UnbookedSwaps(ctx, sqlc.UnbookedSwapsParams{
		After: window.after, Before: window.before, MaxRows: ledgerReconcileRows,
	})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	steps, err := q.SettledBridgeSteps(ctx, sqlc.SettledBridgeStepsParams{After: window.after, Before: window.before})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	missing := make([]unbooked, 0, len(swaps))
	for _, s := range swaps {
		missing = append(missing, unbooked{kind: "swap", id: s.ID.String(), cabalID: s.CabalID, since: s.Since})
	}
	bridged, err := unbookedSteps(ctx, q, steps)
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	missing = append(missing, bridged...)
	for i, m := range missing {
		if i == ledgerReconcileAlerts {
			break
		}
		observability.Alert(ctx, observability.TreasuryLedgerUnbooked,
			slog.String("kind", m.kind), slog.String("id", m.id), slog.String("cabal_id", m.cabalID.String()),
			slog.Time("since", m.since), slog.Int("unbooked_total", len(missing)))
	}
	return poller.Report{Scanned: len(swaps) + len(steps), Changed: len(missing)}, nil
}

func unbookedSteps(ctx context.Context, q *sqlc.Queries, steps []sqlc.SettledBridgeStepsRow) ([]unbooked, error) {
	want := map[uuid.UUID]sqlc.SettledBridgeStepsRow{}
	for _, s := range steps {
		id, ok, err := bridgeTxnID(s)
		if err != nil {
			return nil, err
		}
		if ok {
			want[id] = s
		}
	}
	if len(want) == 0 {
		return nil, nil
	}
	asked := make([]uuid.UUID, 0, len(want))
	for id := range want {
		asked = append(asked, id)
	}
	booked, err := q.BookedCabalTxns(ctx, asked)
	if err != nil {
		return nil, err
	}
	for _, id := range booked {
		delete(want, id)
	}
	out := make([]unbooked, 0, len(want))
	for _, s := range steps {
		id, _, _ := bridgeTxnID(s)
		if _, still := want[id]; still {
			out = append(out, unbooked{
				kind: s.Step + " " + s.Status, id: s.TradeID.String(), cabalID: s.CabalID, since: s.UpdatedAt,
			})
			delete(want, id)
		}
	}
	return out, nil
}

func bridgeTxnID(s sqlc.SettledBridgeStepsRow) (uuid.UUID, bool, error) {
	b := domain.Bridge{
		SwapID: s.TradeID, CabalID: ids.CabalIDFrom(s.CabalID), Direction: domain.BridgeDirection(s.Step),
		Network: chain.Network(s.Network), Attempt: int(s.Attempt),
	}
	if s.Status == "failed" {
		return b.TxnID(s.Status), s.PrivyActionID != "", nil
	}
	swap, err := bookedOn(events.TradeStepChanged{SwapID: s.TradeID, PrivyActionID: s.PrivyActionID})
	b.SwapID = swap
	return b.TxnID(s.Status), err == nil, err
}
