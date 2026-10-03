package tessera

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	tokensPath = "/v1/public/token-details"
	decimals   = 9
)

var _ app.AssetProvider = (*Client)(nil)

type Client struct {
	http *httpclient.Client
}

func New(c *httpclient.Client) *Client { return &Client{http: c} }

func (*Client) Issuer() domain.Issuer { return domain.IssuerTessera }

type tokenWire struct {
	Name   string `json:"name"`
	Code   string `json:"code"`
	Mint   string `json:"mint"`
	Paused bool   `json:"paused"`
}

func (c *Client) Catalog(ctx context.Context) ([]app.ProviderAsset, error) {
	const op = "tessera.Client.Catalog"
	req := (&http.Request{Method: http.MethodGet, URL: &url.URL{Path: tokensPath}, Header: http.Header{}}).
		WithContext(ctx)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	var tokens []tokenWire
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	out := make([]app.ProviderAsset, 0, len(tokens))
	for _, t := range tokens {
		if t.Mint == "" {
			continue
		}
		mint, err := domain.ParseMint(t.Mint)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("symbol", t.Code))
		}
		out = append(out, app.ProviderAsset{
			Symbol: t.Code, Mint: mint, Decimals: decimals, Kind: domain.KindPreIPO, DisplayName: t.Name,
			Tradable: !t.Paused,
		})
	}
	return out, nil
}
