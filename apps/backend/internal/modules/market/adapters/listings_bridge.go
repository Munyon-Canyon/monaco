package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"math/bits"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	bridgeQuotePath = "/v1/deposit_accounts/crypto/quote"
	bridgeMaxBody   = 1 << 16
	microsPerUSDC   = 1_000_000

	bridgeEstimateTTL = 60 * time.Second
)

type PrivyBridgeQuotes struct {
	http   *httpclient.Client
	appID  string
	secret string
}

func NewPrivyBridgeQuotes(c *httpclient.Client, appID, secret string) PrivyBridgeQuotes {
	return PrivyBridgeQuotes{http: c, appID: appID, secret: secret}
}

var _ app.BridgeQuotes = PrivyBridgeQuotes{}

type bridgeLeg struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
}

type bridgeQuoteRequest struct {
	Source      bridgeLeg `json:"source"`
	Destination bridgeLeg `json:"destination"`
	InputAmount string    `json:"input_amount"`
}

type bridgeQuoteWire struct {
	EstimatedOutputAmount string `json:"estimated_output_amount"`
}

func (b PrivyBridgeQuotes) BridgeOut(ctx context.Context, to chain.Network, amount money.Micros) (money.Micros, error) {
	const op = "market.PrivyBridgeQuotes.BridgeOut"
	attrs := []slog.Attr{slog.String("network", string(to)), slog.String("amount_micros", amount.String())}
	raw, _ := json.Marshal(bridgeQuoteRequest{
		Source:      bridgeLeg{Chain: string(chain.NetworkSolana), Asset: "usdc"},
		Destination: bridgeLeg{Chain: string(to), Asset: strings.ToLower(to.Cash().Symbol)},
		InputAmount: new(
			big.Rat,
		).SetFrac(new(big.Int).SetUint64(amount.Uint64()), big.NewInt(microsPerUSDC)).
			FloatString(6),
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, bridgeQuotePath, bytes.NewReader(raw))
	req.SetBasicAuth(b.appID, b.secret)
	req.Header.Set("privy-app-id", b.appID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.http.Do(ctx, req)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), op, attrs...)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return money.Micros{}, errs.New(errs.CodeNoRoute, op, append(attrs, slog.Int("status", resp.StatusCode))...)
	}
	var w bridgeQuoteWire
	if err := json.NewDecoder(io.LimitReader(resp.Body, bridgeMaxBody)).Decode(&w); err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeDecodeFailed, op, attrs...)
	}
	out, ok := new(big.Rat).SetString(w.EstimatedOutputAmount)
	if !ok || out.Sign() <= 0 {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op,
			append(attrs, slog.String("estimated_output_amount", w.EstimatedOutputAmount))...)
	}
	micros := new(big.Int).Quo(new(big.Int).Mul(out.Num(), big.NewInt(microsPerUSDC)), out.Denom())
	if !micros.IsUint64() || micros.Uint64() > amount.Uint64() {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op,
			append(attrs, slog.String("estimated_output_amount", w.EstimatedOutputAmount))...)
	}
	return money.MicrosFromUint64(micros.Uint64()), nil
}

type CachedBridgeQuotes struct {
	inner app.BridgeQuotes
	clock clock.Clock
	mu    sync.Mutex
	rates map[bridgeBucket]bridgeRate
}

type bridgeBucket struct {
	network chain.Network
	dollars uint64
}

type bridgeRate struct {
	arrived, sent uint64
	until         time.Time
}

func NewCachedBridgeQuotes(inner app.BridgeQuotes, clk clock.Clock) *CachedBridgeQuotes {
	return &CachedBridgeQuotes{inner: inner, clock: clk, rates: map[bridgeBucket]bridgeRate{}}
}

var _ app.BridgeQuotes = (*CachedBridgeQuotes)(nil)

func bucketOf(to chain.Network, amount money.Micros) bridgeBucket {
	dollars := (amount.Uint64() + microsPerUSDC - 1) / microsPerUSDC
	if dollars <= 1 {
		return bridgeBucket{network: to, dollars: 1}
	}
	return bridgeBucket{network: to, dollars: 1 << bits.Len64(dollars-1)}
}

func (c *CachedBridgeQuotes) BridgeOut(
	ctx context.Context,
	to chain.Network,
	amount money.Micros,
) (money.Micros, error) {
	key := bucketOf(to, amount)
	if rate, ok := c.cached(key); ok {
		arrived, _ := money.MulDiv(amount.Uint64(), rate.arrived, rate.sent)
		return money.MicrosFromUint64(arrived), nil
	}
	arrived, err := c.inner.BridgeOut(ctx, to, amount)
	if err != nil {
		return money.Micros{}, err
	}
	if !amount.IsZero() && arrived.Cmp(amount) <= 0 {
		c.remember(key, bridgeRate{arrived: arrived.Uint64(), sent: amount.Uint64()})
	}
	return arrived, nil
}

func (c *CachedBridgeQuotes) cached(key bridgeBucket) (bridgeRate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rate, ok := c.rates[key]
	return rate, ok && c.clock.Now().Before(rate.until)
}

func (c *CachedBridgeQuotes) remember(key bridgeBucket, rate bridgeRate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	for k, r := range c.rates {
		if !now.Before(r.until) {
			delete(c.rates, k)
		}
	}
	rate.until = now.Add(bridgeEstimateTTL)
	c.rates[key] = rate
}
