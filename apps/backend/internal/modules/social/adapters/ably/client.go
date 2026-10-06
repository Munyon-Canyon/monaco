package ably

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	ablysdk "github.com/ably/ably-go/ably"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const upstream = "ably"

type Client struct {
	rest *ablysdk.REST
}

type transport struct{ http *httpclient.Client }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.http.Do(r.Context(), r)
}

func New(cfg config.Config, newHTTP func(name string, opts ...httpclient.Option) *httpclient.Client) (Client, error) {
	const op = "ably.New"
	base := cfg.Ably.RESTHost
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Client{}, errs.New(errs.CodeInvalidInput, op)
	}
	hc := newHTTP(upstream, httpclient.WithBaseURL(base), httpclient.WithTimeout(cfg.Timeouts.Ably))
	rest, err := ablysdk.NewREST(
		ablysdk.WithKey(cfg.Ably.APIKey),
		ablysdk.WithFallbackHosts([]string{}),
		ablysdk.WithHTTPClient(&http.Client{Transport: transport{http: hc}}),
		ablysdk.WithUseBinaryProtocol(false),
		ablysdk.WithLogLevel(ablysdk.LogNone),
	)
	if err == nil {
		_, err = rest.Auth.CreateTokenRequest(&ablysdk.TokenParams{})
	}
	if err != nil {
		return Client{}, errs.New(errs.CodeInvalidInput, op)
	}
	return Client{rest: rest}, nil
}

func (c Client) Publish(ctx context.Context, channel string, name string, data any) error {
	const op = "ably.Client.Publish"
	if err := c.rest.Channels.Get(channel).Publish(ctx, name, data); err != nil {
		return upstreamError(err, op, slog.String("channel", channel), slog.String("event", name))
	}
	return nil
}

func (c Client) TokenRequest(
	_ context.Context, clientID ids.UserID, channels []string, ttl time.Duration,
) (app.TokenRequest, error) {
	capability := make(map[string][]string, len(channels))
	for _, channel := range channels {
		capability[channel] = []string{"subscribe"}
	}
	raw, _ := json.Marshal(capability)
	req, _ := c.rest.Auth.CreateTokenRequest(&ablysdk.TokenParams{
		TTL: ttl.Milliseconds(), Capability: string(raw), ClientID: clientID.String(),
	})
	return app.TokenRequest{
		KeyName: req.KeyName, ClientID: req.ClientID, Capability: req.Capability, Timestamp: req.Timestamp,
		TTL: req.TTL, Nonce: req.Nonce, MAC: req.MAC,
	}, nil
}

func upstreamError(err error, op string, attrs ...slog.Attr) error {
	code := errs.CodeUpstreamUnavailable
	var inner *errs.Error
	if errors.As(err, &inner) && inner.Code == errs.CodeUpstreamTimeout {
		code = errs.CodeUpstreamTimeout
	}
	return errs.Wrap(err, code, op, attrs...)
}

type Noop struct{}

func (Noop) Publish(context.Context, string, string, any) error { return nil }

func (Noop) TokenRequest(context.Context, ids.UserID, []string, time.Duration) (app.TokenRequest, error) {
	return app.TokenRequest{}, errs.New(errs.CodeUpstreamUnavailable, "ably.Noop.TokenRequest")
}
