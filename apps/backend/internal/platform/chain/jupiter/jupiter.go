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
	attempts     = 3
	retryBase    = 250 * time.Millisecond
	maxResetWait = 15 * time.Second
)

type Client struct {
	swap    *httpclient.Client
	execute *httpclient.Client
	price   *httpclient.Client
	apiKey  string
	clock   clock.Clock
	window  time.Duration
	pace    *pace
}

func New(cfg config.Config, clk clock.Clock, opts ...httpclient.Option) *Client {
	upstream := func(name, base string, timeout time.Duration) *httpclient.Client {
		return httpclient.New(name, append([]httpclient.Option{
			httpclient.WithBaseURL(base),
			httpclient.WithTimeout(timeout),
			httpclient.WithReturned(http.StatusTooManyRequests),
		}, opts...)...)
	}
	return &Client{
		swap:    upstream("jupiter-swap", cfg.Jupiter.SwapBaseURL, cfg.Timeouts.JupiterQuote),
		execute: upstream("jupiter-execute", cfg.Jupiter.SwapBaseURL, cfg.Timeouts.JupiterExecute),
		price:   upstream("jupiter-price", cfg.Jupiter.PriceBaseURL, cfg.Timeouts.JupiterQuote),
		apiKey:  cfg.Jupiter.APIKey,
		clock:   clk,
		window:  cfg.Timeouts.JupiterExecute,
		pace:    newPace(clk, int(cfg.Jupiter.RateLimit)),
	}
}

type reply struct {
	status  int
	body    []byte
	resetIn time.Duration
}

func (c *Client) call(ctx context.Context, hc *httpclient.Client, l lane, req *http.Request, op string) (reply, error) {
	req.Header.Set("x-api-key", c.apiKey)
	for attempt := 1; ; attempt++ {
		if err := c.pace.take(ctx, l); err != nil {
			return reply{}, err
		}
		r, retryable, err := c.send(ctx, hc, req, op)
		if !retryable || attempt == attempts {
			if err == nil && r.status == http.StatusTooManyRequests {
				return reply{}, errs.New(errs.CodeJupiterUnavailable, op, slog.Int("status", r.status))
			}
			return r, err
		}
		delay := retryBase << (attempt - 1)
		if r.resetIn > 0 {
			delay = r.resetIn
		}
		select {
		case <-ctx.Done():
			return reply{}, errs.Wrap(context.Cause(ctx), errs.CodeUpstreamTimeout, op)
		case <-c.clock.After(delay):
		}
	}
}

func (c *Client) send(ctx context.Context, hc *httpclient.Client, req *http.Request, op string) (reply, bool, error) {
	resp, err := hc.Do(ctx, req)
	if err != nil {
		if errs.CodeOf(err) == errs.CodeUpstreamTimeout {
			return reply{}, false, err
		}
		return reply{}, true, errs.Wrap(err, errs.CodeJupiterUnavailable, op)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return reply{}, false, errs.Wrap(err, errs.CodeJupiterUnavailable, op)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return reply{}, false, errs.New(errs.CodeJupiterUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return reply{status: resp.StatusCode, resetIn: c.resetIn(resp.Header)}, true, nil
	}
	return reply{status: resp.StatusCode, body: body}, false, nil
}

func (c *Client) resetIn(h http.Header) time.Duration {
	secs, err := strconv.ParseInt(h.Get("x-ratelimit-reset"), 10, 64)
	if err != nil {
		return 0
	}
	return min(max(time.Unix(secs, 0).Sub(c.clock.Now()), 0), maxResetWait)
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
