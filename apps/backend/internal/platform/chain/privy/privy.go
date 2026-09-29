package privy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
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
	authKey   *ecdsa.PrivateKey
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
		authKey:   authorizationKey(cfg.Privy.AuthorizationPrivateKey),
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

func authorizationKey(raw string) *ecdsa.PrivateKey {
	der, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(raw), "wallet-auth:"))
	if err != nil {
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil
	}
	priv, _ := key.(*ecdsa.PrivateKey)
	return priv
}

type call struct {
	op      string
	method  string
	path    string
	body    any
	headers map[string]string
}

func (c *Client) do(ctx context.Context, in call, into any) error {
	var body io.Reader
	if in.body != nil {
		raw, _ := json.Marshal(in.body)
		body = bytes.NewReader(raw)
	}
	req, _ := http.NewRequestWithContext(ctx, in.method, in.path, body)
	req.SetBasicAuth(c.cfg.AppID, c.cfg.AppSecret)
	req.Header.Set("privy-app-id", c.cfg.AppID)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range in.headers {
		req.Header.Set(k, v)
	}
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
