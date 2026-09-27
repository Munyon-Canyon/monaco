package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"

	"go.opentelemetry.io/contrib/bridges/otelslog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func NewLogger(cfg config.Config, w io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Env == config.EnvLocal || cfg.Env == config.EnvTest {
		level = slog.LevelDebug
	}
	return slog.New(&handler{level: level, sinks: []slog.Handler{
		slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}),
		otelslog.NewHandler("github.com/monaco/monaco/apps/backend"),
	}})
}

type handler struct {
	level  slog.Level
	sinks  []slog.Handler
	groups []group
}

type group struct {
	name  string
	attrs []slog.Attr
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, redact(a))
		return true
	})
	for i := len(h.groups) - 1; i >= 0; i-- {
		g := h.groups[i]
		attrs = []slog.Attr{{Key: g.name, Value: slog.GroupValue(append(slices.Clone(g.attrs), attrs...)...)}}
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(joinKeys(ctx)...)
	out.AddAttrs(attrs...)
	var failed []error
	for _, sink := range h.sinks {
		if sink.Enabled(ctx, out.Level) {
			if err := sink.Handle(ctx, out); err != nil {
				failed = append(failed, err)
			}
		}
	}
	if err := errors.Join(failed...); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "observability.handler.Handle")
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := redactAll(attrs)
	if len(h.groups) == 0 {
		sinks := make([]slog.Handler, len(h.sinks))
		for i, sink := range h.sinks {
			sinks[i] = sink.WithAttrs(redacted)
		}
		return &handler{level: h.level, sinks: sinks}
	}
	groups := slices.Clone(h.groups)
	last := &groups[len(groups)-1]
	last.attrs = append(slices.Clone(last.attrs), redacted...)
	return &handler{level: h.level, sinks: h.sinks, groups: groups}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{level: h.level, sinks: h.sinks, groups: append(slices.Clone(h.groups), group{name: name})}
}
