package privy

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const maxBody = 1 << 20

type UserID string

type Client struct {
	api       *httpclient.Client
	cfg       config.Privy
	clock     clock.Clock
	verifyKey *ecdsa.PublicKey
}

func New(cfg config.Config, clk clock.Clock, opts ...httpclient.Option) *Client {
	return &Client{
		api: httpclient.New("privy", append([]httpclient.Option{
			httpclient.WithBaseURL(cfg.Privy.BaseURL),
			httpclient.WithTimeout(cfg.Timeouts.Privy),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		}, opts...)...),
		cfg:       cfg.Privy,
		clock:     clk,
		verifyKey: publicKey(cfg.Privy.VerificationKey),
	}
}

func publicKey(raw string) *ecdsa.PublicKey {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(raw, `\n`, "\n")))
	if block == nil {
		return nil
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	pub, _ := key.(*ecdsa.PublicKey)
	return pub
}

type call struct {
	op     string
	method string
	path   string
}

func (c *Client) do(ctx context.Context, in call, into any) error {
	req, _ := http.NewRequestWithContext(ctx, in.method, in.path, nil)
	req.SetBasicAuth(c.cfg.AppID, c.cfg.AppSecret)
	req.Header.Set("privy-app-id", c.cfg.AppID)
	resp, err := c.api.Do(ctx, req)
	if err != nil {
		code := errs.CodePrivyUnavailable
		if errs.CodeOf(err) == errs.CodeUpstreamTimeout {
			code = errs.CodeUpstreamTimeout
		}
		return errs.New(code, in.op, errs.Detail(err)...)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return errs.New(errs.CodePrivyUnavailable, in.op, slog.Int("status", resp.StatusCode))
	}
	if code, failed := statusCode(resp.StatusCode); failed {
		return errs.New(code, in.op, slog.Int("status", resp.StatusCode))
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, in.op, slog.Int("status", resp.StatusCode))
	}
	return nil
}

func statusCode(status int) (errs.Code, bool) {
	switch {
	case status < http.StatusBadRequest:
		return "", false
	case status == http.StatusNotFound:
		return errs.CodeNotFound, true
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return errs.CodeInternal, true
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return errs.CodePrivyUnavailable, true
	}
	return errs.CodeInvalidInput, true
}
