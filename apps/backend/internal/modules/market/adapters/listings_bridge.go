package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	bridgeQuotePath = "/v1/deposit_accounts/crypto/quote"
	bridgeMaxBody   = 1 << 16
	microsPerUSDC   = 1_000_000
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
		Destination: bridgeLeg{Chain: string(to), Asset: "usdc"},
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
