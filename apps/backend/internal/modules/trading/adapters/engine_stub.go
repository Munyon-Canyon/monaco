package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const stubbedCode = "stubbed"

type StubEngine struct {
	UoW *db.UnitOfWork
}

func (s StubEngine) Handle(ctx context.Context, d bus.Delivery, ev events.ProposalPassed) error {
	claimed := false
	err := s.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		claimed, err = d.RecordAs(ctx, tx, stubbedCode)
		return err
	})
	if err != nil || !claimed {
		return err
	}
	observability.Info(ctx, observability.TradingEngineStubbed,
		slog.String("proposal_id", ev.ProposalID.String()), slog.String("kind", ev.Kind),
		slog.String("symbol", ev.Symbol), slog.String("usdc_micros", ev.USDCMicros.String()))
	return nil
}
