package observability

import (
	"context"
	"io"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func NewLogger(cfg config.Config, w io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Env == config.EnvLocal || cfg.Env == config.EnvTest {
		level = slog.LevelDebug
	}
	return slog.New(&handler{next: slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

type handler struct {
	next   slog.Handler
	groups []group
}

type group struct {
	name  string
	attrs []slog.Attr
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
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
	if err := h.next.Handle(ctx, out); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "observability.handler.Handle")
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := redactAll(attrs)
	if len(h.groups) == 0 {
		return &handler{next: h.next.WithAttrs(redacted)}
	}
	groups := slices.Clone(h.groups)
	last := &groups[len(groups)-1]
	last.attrs = append(slices.Clone(last.attrs), redacted...)
	return &handler{next: h.next, groups: groups}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{next: h.next, groups: append(slices.Clone(h.groups), group{name: name})}
}
