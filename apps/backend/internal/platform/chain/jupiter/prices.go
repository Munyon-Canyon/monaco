package jupiter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

type Price struct {
	Mint       Mint
	USDMicros  money.Micros
	ObservedAt time.Time
}

type priceWire struct {
	USDPrice  json.RawMessage `json:"usdPrice"`
	StockData *struct {
		Price json.RawMessage `json:"price"`
	} `json:"stockData"`
}

func (w priceWire) price() json.Number {
	if len(w.USDPrice) == 0 && w.StockData != nil {
		return unquotedNumber(w.StockData.Price)
	}
	return unquotedNumber(w.USDPrice)
}

func unquotedNumber(raw json.RawMessage) json.Number {
	return json.Number(strings.Trim(string(raw), `"`))
}

const (
	pricesPerCall = 50
	priceCalls    = 2
)

type batchAnswer struct {
	prices map[Mint]Price
	err    error
}

func (c *Client) Prices(ctx context.Context, mints []Mint) (map[Mint]Price, error) {
	batches := slices.Collect(slices.Chunk(mints, pricesPerCall))
	answers, err := concurrency.FanOut(ctx, priceCalls, batches, c.answerBatch)
	if err != nil {
		return nil, err
	}
	out := make(map[Mint]Price, len(mints))
	failed := make([]error, 0, len(answers))
	for _, a := range answers {
		maps.Copy(out, a.prices)
		if a.err != nil {
			failed = append(failed, a.err)
		}
	}
	if len(failed) > 0 {
		return out, errs.Wrap(errors.Join(failed...), errs.CodeOf(failed[0]), "jupiter.Prices",
			slog.Int("failed_batches", len(failed)), slog.Int("batches", len(batches)))
	}
	return out, nil
}

func (c *Client) answerBatch(ctx context.Context, batch []Mint) (batchAnswer, error) {
	prices, err := c.priceBatch(ctx, batch)
	return batchAnswer{prices: prices, err: err}, nil
}

func (c *Client) priceBatch(ctx context.Context, batch []Mint) (map[Mint]Price, error) {
	const op = "jupiter.Prices"
	ids := make([]string, len(batch))
	for i, m := range batch {
		ids[i] = m.Address
	}
	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"?"+url.Values{"ids": {strings.Join(ids, ",")}}.Encode(),
		nil,
	)
	r, err := c.call(ctx, c.price, req, op)
	if err != nil {
		return nil, err
	}
	if r.status != http.StatusOK {
		return nil, rejected(op, r.status, 0, "")
	}
	var wire map[string]*priceWire
	if err := decode(r, op, &wire); err != nil {
		return nil, err
	}
	observed := c.clock.Now()
	out := make(map[Mint]Price, len(batch))
	for _, m := range batch {
		w := wire[m.Address]
		if w == nil {
			continue
		}
		usd, ok := parsePrice(w.price())
		if !ok {
			var stock json.RawMessage
			if w.StockData != nil {
				stock = w.StockData.Price
			}
			boundary.Warn(ctx, observability.JupiterPriceSkipped, slog.String("mint", m.Address),
				slog.String("usd_price", string(w.USDPrice)), slog.String("stock_price", string(stock)))
			continue
		}
		out[m] = Price{Mint: m, USDMicros: usd, ObservedAt: observed}
	}
	return out, nil
}

func parsePrice(n json.Number) (money.Micros, bool) {
	r, ok := decimal(n)
	if !ok || r.Sign() < 0 {
		return money.Micros{}, false
	}
	r.Mul(r, big.NewRat(1_000_000, 1))
	micros := new(big.Int).Quo(r.Num(), r.Denom())
	return money.MicrosFromUint64(micros.Uint64()), micros.IsUint64()
}
