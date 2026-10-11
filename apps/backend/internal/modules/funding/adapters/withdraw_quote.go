package adapters

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ app.TransferQuotes = PrivyTransferQuotes{}

const maxQuoteLife = 5 * time.Minute

type PrivyTransferQuotes struct {
	cfg   config.Config
	clock clock.Clock
	opts  []httpclient.Option
}

func NewPrivyTransferQuotes(cfg config.Config, clk clock.Clock, opts ...httpclient.Option) PrivyTransferQuotes {
	return PrivyTransferQuotes{cfg: cfg, clock: clk, opts: opts}
}

func (q PrivyTransferQuotes) client(op string) (*httpclient.Client, error) {
	if q.cfg.Privy.BaseURL == "" || q.cfg.Timeouts.Privy <= 0 {
		return nil, errs.New(
			errs.CodeInternal,
			op,
			slog.String("reason", "PRIVY_BASE_URL or MONACO_TIMEOUT_PRIVY missing"),
		)
	}
	return httpclient.New("privy-transfer-quote", append([]httpclient.Option{
		httpclient.WithBaseURL(q.cfg.Privy.BaseURL),
		httpclient.WithTimeout(q.cfg.Timeouts.Privy),
		httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
	}, q.opts...)...), nil
}

type quoteAsset struct {
	Asset   string `json:"asset"`
	Chain   string `json:"chain"`
	Address string `json:"address,omitempty"`
}

type quoteRequest struct {
	Amount      string     `json:"amount"`
	AmountType  string     `json:"amount_type"`
	Source      quoteAsset `json:"source"`
	Destination quoteAsset `json:"destination"`
}

type quoteResponse struct {
	EstimatedOutput string `json:"estimated_output_amount"`
	ExpiresAt       int64  `json:"expires_at"`
}

func (q PrivyTransferQuotes) QuoteTransfer(
	ctx context.Context, walletID string, spec multichain.TransferSpec,
) (app.TransferQuote, error) {
	const op = "funding.PrivyTransferQuotes.QuoteTransfer"
	api, err := q.client(op)
	if err != nil {
		return app.TransferQuote{}, err
	}
	raw, _ := json.Marshal(quoteRequest{
		Amount: usdcDecimal(spec.AmountMicros), AmountType: "exact_input",
		Source:      quoteAsset{Asset: "usdc", Chain: spec.From.PrivyChain()},
		Destination: quoteAsset{Asset: "usdc", Chain: spec.To.PrivyChain(), Address: spec.Destination},
	})
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/transfer/quote"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(raw))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	req.SetBasicAuth(q.cfg.Privy.AppID, q.cfg.Privy.AppSecret)
	req.Header.Set("privy-app-id", q.cfg.Privy.AppID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.Do(ctx, req)
	if err != nil {
		return app.TransferQuote{}, errs.Wrap(err, errs.CodePrivyUnavailable, op)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= http.StatusBadRequest {
		return app.TransferQuote{}, errs.New(
			quoteStatusCode(reply),
			op,
			slog.Int("status", resp.StatusCode),
		)
	}
	var out quoteResponse
	if err := json.Unmarshal(reply, &out); err != nil {
		return app.TransferQuote{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	receive, err := parseUSDCDecimal(out.EstimatedOutput)
	expires, now := time.Unix(out.ExpiresAt, 0).UTC(), q.clock.Now()
	if err != nil || !expires.After(now) || expires.After(now.Add(maxQuoteLife)) {
		return app.TransferQuote{}, errs.New(errs.CodeDecodeFailed, op,
			slog.String("estimated_output_amount", out.EstimatedOutput), slog.Int64("expires_at", out.ExpiresAt))
	}
	return app.TransferQuote{ReceiveMicros: receive, ExpiresAt: expires}, nil
}

func quoteStatusCode(body []byte) errs.Code {
	if bytes.Contains(body, []byte("Insufficient balance")) {
		return errs.CodeInsufficientFunds
	}
	return errs.CodeInvalidInput
}

func usdcDecimal(m money.Micros) string {
	whole, frac := m.Uint64()/1_000_000, m.Uint64()%1_000_000
	if frac == 0 {
		return strconv.FormatUint(whole, 10)
	}
	return strconv.FormatUint(whole, 10) + "." + strings.TrimRight(strconv.FormatUint(frac+1_000_000, 10)[1:], "0")
}

func parseUSDCDecimal(raw string) (money.Micros, error) {
	whole, frac, _ := strings.Cut(raw, ".")
	frac = (frac + "000000")[:6]
	return money.ParseMicros(cmp.Or(strings.TrimLeft(whole+frac, "0"), "0"))
}
