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
	"strconv"
	"strings"
	"sync/atomic"
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
	USDPrice  wireNumber `json:"usdPrice"`
	StockData *struct {
		Price wireNumber `json:"price"`
	} `json:"stockData"`
}

type wireNumber string

func (n *wireNumber) UnmarshalJSON(raw []byte) error {
	var quoted string
	if json.Unmarshal(raw, &quoted) == nil {
		*n = wireNumber(quoted)
		return nil
	}
	*n = wireNumber(raw)
	return nil
}

func (w priceWire) price() json.Number {
	if w.USDPrice == "" && w.StockData != nil {
		return json.Number(w.StockData.Price)
	}
	return json.Number(w.USDPrice)
}

const (
	pricesPerCall = 50
	priceCalls    = 4
)

type batchAnswer struct {
	prices map[Mint]Price
	err    error
}

func (c *Client) PriceCapacity(window time.Duration) int {
	windows := max(int(window/paceWindow)-1, 1)
	return windows * c.pace.laneLimit(priceLane) * pricesPerCall
}

func (c *Client) Prices(ctx context.Context, mints []Mint) (map[Mint]Price, error) {
	batches := slices.Collect(slices.Chunk(mints, pricesPerCall))
	var throttled atomic.Pointer[error]
	answer := func(_ context.Context, batch []Mint) (batchAnswer, error) {
		return c.answerBatch(ctx, batch, &throttled), nil
	}
	answers, _ := concurrency.FanOut(context.WithoutCancel(ctx), priceCalls, batches, answer)
	out := make(map[Mint]Price, len(mints))
	var failed []error
	seen := map[string]bool{}
	failedBatches := 0
	for _, a := range answers {
		maps.Copy(out, a.prices)
		if a.err == nil {
			continue
		}
		failedBatches++
		if kind := string(errs.CodeOf(a.err)) + strconv.FormatInt(attrStatus(a.err), 10); !seen[kind] {
			seen[kind] = true
			failed = append(failed, a.err)
		}
	}
	if failedBatches > 0 {
		return out, errs.Wrap(errors.Join(failed...), errs.CodeOf(failed[0]), "jupiter.Prices",
			slog.Int("failed_batches", failedBatches), slog.Int("batches", len(batches)))
	}
	return out, nil
}

func attrStatus(err error) int64 {
	for _, a := range errs.Detail(err) {
		if a.Key == "status" {
			return a.Value.Int64()
		}
	}
	return 0
}

func (c *Client) answerBatch(ctx context.Context, batch []Mint, throttled *atomic.Pointer[error]) batchAnswer {
	if stop := throttled.Load(); stop != nil {
		return batchAnswer{err: *stop}
	}
	if err := ctx.Err(); err != nil {
		return batchAnswer{err: errs.Wrap(context.Cause(ctx), errs.CodeUpstreamTimeout, "jupiter.Prices")}
	}
	prices, err := c.priceBatch(ctx, batch)
	if err != nil && attrStatus(err) == http.StatusTooManyRequests {
		throttled.CompareAndSwap(nil, &err)
	}
	return batchAnswer{prices: prices, err: err}
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
	r, err := c.call(ctx, c.price, priceLane, req, op)
	if err != nil {
		return nil, err
	}
	if r.status != http.StatusOK {
		return nil, rejected(op, r.status, 0, "")
	}
	var wire map[string]json.RawMessage
	if err := decode(r, op, &wire); err != nil {
		return nil, err
	}
	observed := c.clock.Now()
	out := make(map[Mint]Price, len(batch))
	for _, m := range batch {
		raw, found := wire[m.Address]
		if !found || string(raw) == "null" {
			continue
		}
		var w priceWire
		usd, ok := money.Micros{}, json.Unmarshal(raw, &w) == nil
		if ok {
			usd, ok = parsePrice(w.price())
		}
		if !ok {
			var stock wireNumber
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
