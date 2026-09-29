package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	maxBody     = 4 << 20
	mintTTL     = time.Hour
	maxStatuses = 256
)

type Client struct {
	rpc   *httpclient.Client
	path  string
	clock clock.Clock
	mu    sync.Mutex
	mints map[chain.SolanaAddress]cachedMint
}

type cachedMint struct {
	cfg MintConfig
	at  time.Time
}

func New(cfg config.Config, clk clock.Clock, opts ...httpclient.Option) *Client {
	base, query, _ := strings.Cut(cfg.Solana.RPCURL, "?")
	path := "/"
	if query != "" {
		path += "?" + query
	}
	return &Client{
		rpc: httpclient.New("solana-rpc", append([]httpclient.Option{
			httpclient.WithBaseURL(base),
			httpclient.WithTimeout(cfg.Timeouts.RPC),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		}, opts...)...),
		path:  path,
		clock: clk,
		mints: map[chain.SolanaAddress]cachedMint{},
	}
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params []any, into any) error {
	op := "solana." + method
	body, _ := json.Marshal(request{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.rpc.Do(ctx, req)
	if err != nil {
		code := errs.CodeRPCUnavailable
		if errs.CodeOf(err) == errs.CodeUpstreamTimeout {
			code = errs.CodeUpstreamTimeout
		}
		return errs.New(code, op, errs.Detail(err)...)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return errs.New(errs.CodeRPCUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeRPCUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	if r.Error != nil {
		return errs.New(errs.CodeRPCUnavailable, op,
			slog.Int("rpc_code", r.Error.Code), slog.String("rpc_message", r.Error.Message))
	}
	if err := json.Unmarshal(r.Result, into); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return nil
}

func addresses(op string, addrs ...chain.SolanaAddress) error {
	for _, a := range addrs {
		if _, err := chain.ParseAddress(string(a)); err != nil {
			return errs.Wrap(err, errs.CodeInvalidAddress, op)
		}
	}
	return nil
}
