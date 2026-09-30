package xstocks

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	assetsPath     = "/api/v2/public/assets"
	solanaNetwork  = "Solana"
	solanaDecimals = 8
	maxPages       = 64
)

var _ app.AssetProvider = (*Client)(nil)

type Client struct {
	http *httpclient.Client
}

func New(c *httpclient.Client) *Client { return &Client{http: c} }

func (*Client) Issuer() domain.Issuer { return domain.IssuerXStocks }

type listWire struct {
	Nodes []nodeWire `json:"nodes"`
	Page  struct {
		HasNextPage bool `json:"hasNextPage"`
	} `json:"page"`
}

type nodeWire struct {
	Symbol          string           `json:"symbol"`
	Name            string           `json:"name"`
	Logo            string           `json:"logo"`
	IsTradingHalted bool             `json:"isTradingHalted"`
	Deployments     []deploymentWire `json:"deployments"`
}

type deploymentWire struct {
	Address string `json:"address"`
	Network string `json:"network"`
}

func (c *Client) Catalog(ctx context.Context) ([]app.ProviderAsset, error) {
	var out []app.ProviderAsset
	for page := range maxPages {
		list, err := c.page(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, n := range list.Nodes {
			asset, listed, err := toProviderAsset(n)
			if err != nil {
				return nil, err
			}
			if listed {
				out = append(out, asset)
			}
		}
		if !list.Page.HasNextPage {
			return out, nil
		}
	}
	return nil, errs.New(errs.CodeDecodeFailed, "xstocks.Client.Catalog", slog.Int("max_pages", maxPages))
}

func (c *Client) page(ctx context.Context, page int) (listWire, error) {
	const op = "xstocks.Client.page"
	req := (&http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: assetsPath, RawQuery: "page=" + strconv.Itoa(page)},
		Header: http.Header{},
	}).WithContext(ctx)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return listWire{}, errs.Wrap(err, errs.CodeOf(err), op, slog.Int("page", page))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return listWire{}, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("page", page),
			slog.Int("status", resp.StatusCode))
	}
	var w listWire
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		return listWire{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.Int("page", page))
	}
	return w, nil
}

func toProviderAsset(n nodeWire) (app.ProviderAsset, bool, error) {
	for _, d := range n.Deployments {
		if d.Network != solanaNetwork {
			continue
		}
		mint, err := domain.ParseMint(d.Address)
		if err != nil {
			return app.ProviderAsset{}, false, errs.Wrap(err, errs.CodeDecodeFailed, "xstocks.toProviderAsset",
				slog.String("symbol", n.Symbol))
		}
		return app.ProviderAsset{
			Symbol:      n.Symbol,
			Mint:        mint,
			Decimals:    solanaDecimals,
			Kind:        domain.KindEquity,
			DisplayName: n.Name,
			LogoURL:     httpsOnly(n.Logo),
			Tradable:    !n.IsTradingHalted,
		}, true, nil
	}
	return app.ProviderAsset{}, false, nil
}

func httpsOnly(raw string) string {
	if strings.HasPrefix(raw, "https://") {
		return raw
	}
	return ""
}
