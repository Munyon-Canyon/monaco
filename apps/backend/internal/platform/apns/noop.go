package apns

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type NoopSender struct{}

var _ Sender = NoopSender{}

func (NoopSender) Send(ctx context.Context, p Push) (Result, error) {
	observability.Info(ctx, observability.APNSNoopSend, slog.String("user_id", p.UserID.String()))
	return Result{Status: http.StatusOK}, nil
}
