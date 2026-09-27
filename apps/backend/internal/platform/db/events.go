package db

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/propagation"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Events struct {
	q        *sqlc.Queries
	ids      ids.Generator
	clock    clock.Clock
	appended []uuid.UUID
}

func (e *Events) Append(ctx context.Context, ev events.Event) error {
	const op = "db.Events.Append"
	actor := observability.ActorFrom(ctx)
	actorType, actorID, ok := strings.Cut(actor, ":")
	if !ok || actorType == "" || actorID == "" {
		return errs.New(errs.CodeInternal, op, slog.String("actor", actor))
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op, slog.String("type", string(ev.Type())))
	}
	id := e.ids.NewV7()
	err = e.q.AppendEvent(ctx, sqlc.AppendEventParams{
		ID:            id,
		AggregateType: ev.AggregateType(),
		AggregateID:   ev.AggregateID(),
		Type:          string(ev.Type()),
		Payload:       payload,
		ActorType:     actorType,
		ActorID:       actorID,
		TraceParent:   traceParent(ctx),
		CreatedAt:     e.clock.Now(),
	})
	if err != nil {
		return classify(err, op)
	}
	e.appended = append(e.appended, id)
	return nil
}

func traceParent(ctx context.Context) pgtype.Text {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	tp := carrier.Get("traceparent")
	return pgtype.Text{String: tp, Valid: tp != ""}
}
