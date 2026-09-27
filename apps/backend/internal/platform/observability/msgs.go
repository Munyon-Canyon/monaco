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
)

var registry = []Msg{
	BootConfig,
	BootListening,
	BootStopped,
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
