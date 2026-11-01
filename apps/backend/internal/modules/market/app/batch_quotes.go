package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

const quotesMax = 50

type BatchQuoter interface {
	Handle(context.Context, []string) ([]Summary, error)
}

type quotesReader interface {
	AssetsBySymbols(context.Context, []string) ([]sqlc.Asset, error)
}

var _ BatchQuoter = (*BatchQuotes)(nil)

var _ quotesReader = (*sqlc.Queries)(nil)

type BatchQuotes struct {
	list *ListAssets
	read quotesReader
}

func NewBatchQuotes(db sqlc.DBTX, list *ListAssets) *BatchQuotes {
	return &BatchQuotes{list: list, read: sqlc.New(db)}
}

func (q *BatchQuotes) Handle(ctx context.Context, symbols []string) ([]Summary, error) {
	wanted, err := quoteSymbols(symbols)
	if err != nil {
		return nil, err
	}
	rows, err := q.read.AssetsBySymbols(ctx, wanted)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.AssetsBySymbols")
	}
	if len(rows) == 0 {
		return []Summary{}, nil
	}
	assets, err := assetsOf(rows)
	if err != nil {
		return nil, err
	}
	return q.list.summaries(ctx, assets, q.list.clock.Now())
}

func quoteSymbols(symbols []string) ([]string, error) {
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}
		if _, dup := seen[symbol]; dup {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	if len(out) == 0 || len(out) > quotesMax {
		return nil, errs.New(errs.CodeInvalidInput, "market.AssetQuotes", slog.Int("symbols", len(out)))
	}
	return out, nil
}
