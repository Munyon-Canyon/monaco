package prestocks

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	listPath = "/api/prestocks"
	decimals = 9
)

var _ app.AssetProvider = (*Client)(nil)

type Client struct {
	http *httpclient.Client
}

func New(c *httpclient.Client) *Client { return &Client{http: c} }

func (*Client) Issuer() domain.Issuer { return domain.IssuerPreStocks }

type tokenWire struct {
	Name            string `json:"name"`
	Symbol          string `json:"symbol"`
	Image           string `json:"image"`
	ContractAddress string `json:"contract_address"`
	Paused          bool   `json:"paused"`
}

func (c *Client) Catalog(ctx context.Context) ([]app.ProviderAsset, error) {
	const op = "prestocks.Client.Catalog"
	req := (&http.Request{Method: http.MethodGet, URL: &url.URL{Path: listPath}, Header: http.Header{}}).
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
		if t.ContractAddress == "" {
			continue
		}
		mint, err := domain.ParseMint(t.ContractAddress)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("symbol", t.Symbol))
		}
		logo := t.Image
		if !strings.HasPrefix(logo, "https://") {
			logo = ""
		}
		out = append(out, app.ProviderAsset{
			Symbol: t.Symbol, Mint: mint, Decimals: decimals, Kind: domain.KindPreIPO, DisplayName: t.Name,
			LogoURL: logo, Tradable: !t.Paused,
		})
	}
	return out, nil
}
