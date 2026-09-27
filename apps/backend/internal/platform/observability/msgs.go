package observability

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/internal/emit"
)

type Msg struct {
	Name     string
	Required []string
}

var (
	BootConfig    = Msg{Name: "boot.config", Required: []string{"service", "config"}}
	BootListening = Msg{Name: "boot.listening", Required: []string{"service", "addr"}}
	BootStopped   = Msg{Name: "boot.stopped", Required: []string{"service", "err"}}
	DBLockLost    = Msg{Name: "db.lock.lost", Required: []string{"lock", "held", "err"}}
	HTTPRetry     = Msg{Name: "httpclient.retry", Required: []string{"upstream", "attempt", "status", "delay"}}
	HTTPRequest   = Msg{Name: "http.request", Required: []string{"method", "route", "status", "duration_ms"}}
	HTTPProblem   = Msg{Name: "http.problem", Required: []string{"code", "status", "err", "alert"}}
	PollerTick    = Msg{Name: "poller.tick", Required: []string{"poller", "scanned", "changed", "duration_ms"}}
	PollerFailed  = Msg{Name: "poller.tick.failed", Required: []string{"poller", "code", "err", "alert"}}
	PollerSkipped = Msg{Name: "poller.tick.skipped_locked", Required: []string{"poller"}}
	TxCommitted   = Msg{Name: "tx.committed", Required: []string{"event_ids", "attempt"}}
	TxRolledBack  = Msg{Name: "tx.rolled_back", Required: []string{"code", "attempt"}}
	TxRetry       = Msg{Name: "tx.retry", Required: []string{"code", "attempt", "delay"}}

	BusRelayTick          = Msg{Name: "bus.relay.tick", Required: []string{"count", "first_id", "last_id"}}
	BusRelayIdle          = Msg{Name: "bus.relay.idle"}
	BusRelayPublishFailed = Msg{Name: "bus.relay.publish_failed", Required: []string{"code", "err"}}
	BusRelayFailed        = Msg{Name: "bus.relay.failed", Required: []string{"code", "err"}}
)

var registry = []Msg{
	BootConfig,
	BootListening,
	BootStopped,
	DBLockLost,
	HTTPRetry,
	HTTPRequest,
	HTTPProblem,
	PollerTick,
	PollerFailed,
	PollerSkipped,
	TxCommitted,
	TxRolledBack,
	TxRetry,
	BusRelayTick,
	BusRelayIdle,
	BusRelayPublishFailed,
	BusRelayFailed,
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return emit.WithLogger(ctx, l)
}

func Info(ctx context.Context, m Msg, attrs ...slog.Attr) {
	emit.Log(ctx, slog.LevelInfo, m.Name, attrs)
}

func Debug(ctx context.Context, m Msg, attrs ...slog.Attr) {
	emit.Log(ctx, slog.LevelDebug, m.Name, attrs)
}

func WriteCatalog(w io.Writer) error {
	msgs := slices.SortedFunc(slices.Values(registry), func(a, b Msg) int { return strings.Compare(a.Name, b.Name) })
	var b strings.Builder
	b.WriteString("| Message | Required attrs |\n| --- | --- |\n")
	for _, m := range msgs {
		attrs := make([]string, len(m.Required))
		for i, k := range m.Required {
			attrs[i] = "`" + k + "`"
		}
		b.WriteString("| `" + m.Name + "` | " + strings.Join(attrs, ", ") + " |\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "observability.WriteCatalog")
	}
	return nil
}
