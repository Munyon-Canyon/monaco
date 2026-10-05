package jupiter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Mint struct {
	Address  string
	Decimals uint8
}

type SolanaAddress string

type Status uint8

const (
	StatusSuccess Status = iota + 1
	StatusFailed
	StatusPending
)

type Order struct {
	RequestID   string
	Transaction []byte
	InMint      Mint
	OutMint     Mint
	InAmount    money.BaseUnits
	OutAmount   money.BaseUnits
	Router      string
}

type Quote struct {
	InAmount       money.BaseUnits
	OutAmount      money.BaseUnits
	PriceImpactBps int64
	Routable       bool
}

type ExecuteResult struct {
	Status    Status
	Signature string
	InAmount  uint64
	OutAmount uint64
	ErrorCode int
}

type OrderSpec struct {
	In, Out     Mint
	Amount      money.BaseUnits
	Taker       SolanaAddress
	Payer       SolanaAddress
	SlippageBps int64
}

type QuoteSpec struct {
	In, Out Mint
	Amount  money.BaseUnits
}

const (
	pendingEvery = 2 * time.Second
	maxBody      = 1 << 20
)

type Client struct {
	swap    *httpclient.Client
	execute *httpclient.Client
	price   *httpclient.Client
	apiKey  string
	clock   clock.Clock
	window  time.Duration
}

func New(cfg config.Config, clk clock.Clock, opts ...httpclient.Option) *Client {
	upstream := func(name, base string, timeout time.Duration) *httpclient.Client {
		return httpclient.New(name, append([]httpclient.Option{
			httpclient.WithBaseURL(base),
			httpclient.WithTimeout(timeout),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		}, opts...)...)
	}
	return &Client{
		swap:    upstream("jupiter-swap", cfg.Jupiter.SwapBaseURL, cfg.Timeouts.JupiterQuote),
		execute: upstream("jupiter-execute", cfg.Jupiter.SwapBaseURL, cfg.Timeouts.JupiterExecute),
		price:   upstream("jupiter-price", cfg.Jupiter.PriceBaseURL, cfg.Timeouts.JupiterQuote),
		apiKey:  cfg.Jupiter.APIKey,
		clock:   clk,
		window:  cfg.Timeouts.JupiterExecute,
	}
}

type reply struct {
	status int
	body   []byte
}

func (c *Client) call(ctx context.Context, hc *httpclient.Client, req *http.Request, op string) (reply, error) {
	req.Header.Set("x-api-key", c.apiKey)
	resp, err := hc.Do(ctx, req)
	if err != nil {
		if errs.CodeOf(err) == errs.CodeUpstreamTimeout {
			return reply{}, err
		}
		return reply{}, errs.Wrap(err, errs.CodeJupiterUnavailable, op)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return reply{}, errs.Wrap(err, errs.CodeJupiterUnavailable, op)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return reply{}, errs.New(errs.CodeJupiterUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	return reply{status: resp.StatusCode, body: body}, nil
}

func decode(r reply, op string, into any) error {
	if err := json.Unmarshal(r.body, into); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, op, slog.Int("status", r.status))
	}
	return nil
}

func rejected(op string, status, code int, message string) error {
	return errs.New(errs.CodeJupiterRejected, op,
		slog.Int("status", status), slog.Int("jupiter_code", code), slog.String("jupiter_message", message))
}

func count(raw, op string) (uint64, error) {
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("amount", raw))
	}
	return v, nil
}

func baseUnits(raw string, m Mint, op string) (money.BaseUnits, error) {
	v, err := count(raw, op)
	return money.NewBaseUnits(v, m.Decimals), err
}

var decimalNumber = regexp.MustCompile(`^-?[0-9]{1,40}(\.[0-9]{1,40})?([eE][-+]?[0-9]{1,3})?$`)

func decimal(n json.Number) (*big.Rat, bool) {
	if !decimalNumber.MatchString(string(n)) {
		return nil, false
	}
	r, _ := new(big.Rat).SetString(string(n))
	return r, true
}
