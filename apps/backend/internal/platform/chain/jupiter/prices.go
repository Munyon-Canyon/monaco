package jupiter

import (
	"context"
	"encoding/json"
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
)

type Price struct {
	Mint       Mint
	USDMicros  money.Micros
	ObservedAt time.Time
}

type priceWire struct {
	USDPrice json.Number `json:"usdPrice"`
}

const (
	pricesPerCall = 50
	priceCalls    = 2
)

func (c *Client) Prices(ctx context.Context, mints []Mint) (map[Mint]Price, error) {
	batches := slices.Collect(slices.Chunk(mints, pricesPerCall))
	got, err := concurrency.FanOut(ctx, priceCalls, batches, c.priceBatch)
	if err != nil {
		return nil, err
	}
	out := make(map[Mint]Price, len(mints))
	for _, batch := range got {
		maps.Copy(out, batch)
	}
	return out, nil
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
		usd, ok := parsePrice(w.USDPrice)
		if !ok {
			return nil, errs.New(errs.CodeDecodeFailed, op,
				slog.String("mint", m.Address), slog.String("usd_price", string(w.USDPrice)))
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
