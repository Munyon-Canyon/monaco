package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const statusClientClosedRequest = 499

func Problem(w http.ResponseWriter, r *http.Request, err error) {
	code := errs.CodeOf(err)
	status := errs.HTTPStatus(errs.KindOf(code))
	if errors.Is(r.Context().Err(), context.Canceled) {
		code, status = errs.CodeClientClosed, statusClientClosedRequest
		observability.Info(r.Context(), observability.HTTPProblem, slog.String("code", string(code)),
			slog.Int("status", status), slog.Any("err", err), slog.Bool("alert", false))
		writeProblem(w, r, code, status)
		return
	}
	logProblem(r.Context(), err, code, status)
	writeProblem(w, r, code, status)
}

func writeProblem(w http.ResponseWriter, r *http.Request, code errs.Code, status int) {
	body := api.Problem{
		Type:      api.AboutBlank,
		Title:     title(status),
		Status:    status,
		Code:      api.ErrorCode(code),
		Message:   errs.Message(code),
		TraceId:   trace.SpanContextFromContext(r.Context()).TraceID().String(),
		Retryable: errs.Retryable(code),
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func logProblem(ctx context.Context, err error, code errs.Code, status int) {
	detail := errs.Detail(err)
	if status >= http.StatusInternalServerError {
		boundary.Error(ctx, observability.HTTPProblem, slog.String("code", string(code)), slog.Int("status", status),
			slog.Any("err", err), slog.Bool("alert", errs.Alert(code)), slog.GroupAttrs("detail", detail...))
		return
	}
	observability.Info(ctx, observability.HTTPProblem, slog.String("code", string(code)), slog.Int("status", status),
		slog.Any("err", err), slog.Bool("alert", errs.Alert(code)), slog.GroupAttrs("detail", detail...))
}

func title(status int) string {
	if status == statusClientClosedRequest {
		return "Client Closed Request"
	}
	return http.StatusText(status)
}
