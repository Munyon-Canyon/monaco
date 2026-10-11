package ondo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	Issuer           = "ondo"
	assetsPath       = "/api/v2/assets"
	assetPagePrefix  = "/assets/"
	pageConcurrency  = 4
	pagesPerSecond   = 10
	maxListBodySize  = 16 << 20
	maxPageBodySize  = 8 << 20
	ethereumNetwork  = "ETHEREUM"
	solanaNetwork    = "SOLANA"
	escapedQuote     = `\"`
	supportedNetsKey = `"supportedNetworks":`
)

var _ app.ListingSource = (*Client)(nil)

type Client struct {
	http  *httpclient.Client
	limit *rate.Limiter
}

func New(c *httpclient.Client) *Client {
	return &Client{http: c, limit: rate.NewLimiter(rate.Every(time.Second/pagesPerSecond), pageConcurrency)}
}

func (*Client) Issuer() string { return Issuer }

type assetsWire struct {
	Assets []assetWire `json:"assets"`
}

type assetWire struct {
	Symbol          string `json:"symbol"`
	Ticker          string `json:"ticker"`
	IsTradingPaused bool   `json:"isTradingPaused"`
}

type networkWire struct {
	Network  string `json:"network"`
	ChainID  int64  `json:"chainId"`
	Address  string `json:"address"`
	Decimals *uint8 `json:"decimals"`
}

type assetPage struct {
	published      bool
	invalid        string
	sharesPerToken *big.Rat
	networks       []networkWire
}

var assetBlock = regexp.MustCompile(`"sharesMultiplier":"([0-9.]+)","symbol":"([^"]*)"[^{}\[\]]*?` + supportedNetsKey)

func (c *Client) Listings(ctx context.Context) (app.ListingBatch, error) {
	const op = "ondo.Client.Listings"
	assets, err := c.assets(ctx)
	if err != nil {
		return app.ListingBatch{}, err
	}
	pages, err := concurrency.FanOut(ctx, pageConcurrency, assets, c.page)
	if err != nil {
		return app.ListingBatch{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	batch := app.ListingBatch{Issuer: Issuer}
	for i, a := range assets {
		page := pages[i]
		if !page.published {
			batch.Missing = append(batch.Missing, a.Symbol)
			continue
		}
		if page.invalid != "" {
			skipped(ctx, a, page.invalid)
			continue
		}
		for _, n := range page.networks {
			l, reason := toListing(a, page.sharesPerToken, n)
			switch {
			case reason != "":
				skipped(ctx, a, reason)
			case l.Network != "":
				batch.Listings = append(batch.Listings, l)
			}
		}
	}
	return batch, nil
}

func skipped(ctx context.Context, a assetWire, reason string) {
	if !a.IsTradingPaused {
		observability.Degraded(ctx, observability.MarketListingAssetSkipped, slog.String("issuer", Issuer),
			slog.String("symbol", a.Symbol), slog.String("reason", reason))
	}
}

func (c *Client) assets(ctx context.Context) ([]assetWire, error) {
	const op = "ondo.Client.assets"
	body, status, err := c.get(ctx, assetsPath, maxListBodySize)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if status != http.StatusOK {
		return nil, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("status", status))
	}
	var w assetsWire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return w.Assets, nil
}

func (c *Client) page(ctx context.Context, a assetWire) (assetPage, error) {
	const op = "ondo.Client.page"
	if err := c.limit.Wait(ctx); err != nil {
		return assetPage{}, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, slog.String("symbol", a.Symbol))
	}
	body, status, err := c.get(ctx, assetPagePrefix+url.PathEscape(strings.ToLower(a.Symbol)), maxPageBodySize)
	if err != nil {
		return assetPage{}, errs.Wrap(err, errs.CodeOf(err), op, slog.String("symbol", a.Symbol))
	}
	switch status {
	case http.StatusOK:
		return parsePage(body, a.Symbol), nil
	case http.StatusNotFound:
		return assetPage{}, nil
	default:
		return assetPage{}, errs.New(errs.CodeUpstreamUnavailable, op,
			slog.String("symbol", a.Symbol), slog.Int("status", status))
	}
}

func (c *Client) get(ctx context.Context, path string, limit int64) ([]byte, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, 0, errs.Wrap(err, errs.CodeOf(err), "ondo.Client.get", slog.String("path", path))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, 0, errs.Wrap(err, errs.CodeUpstreamUnavailable, "ondo.Client.get")
	}
	return body, resp.StatusCode, nil
}

func parsePage(html []byte, symbol string) assetPage {
	payload := bytes.ReplaceAll(html, []byte(escapedQuote), []byte(`"`))
	for _, m := range assetBlock.FindAllSubmatchIndex(payload, -1) {
		if string(payload[m[4]:m[5]]) != symbol {
			continue
		}
		shares, ok := new(big.Rat).SetString(string(payload[m[2]:m[3]]))
		if !ok || shares.Sign() <= 0 {
			return assetPage{published: true, invalid: "bad multiplier"}
		}
		var networks []networkWire
		if err := json.NewDecoder(bytes.NewReader(payload[m[1]:])).Decode(&networks); err != nil {
			return assetPage{published: true, invalid: "bad networks"}
		}
		return assetPage{published: true, sharesPerToken: shares, networks: networks}
	}
	return assetPage{}
}

func toListing(a assetWire, shares *big.Rat, n networkWire) (app.IssuerListing, string) {
	l := app.IssuerListing{
		Issuer:         Issuer,
		Ticker:         a.Ticker,
		Symbol:         a.Symbol,
		Decimals:       n.Decimals,
		SharesPerToken: shares,
		Tradable:       !a.IsTradingPaused,
	}
	switch {
	case n.Network != solanaNetwork && n.Network != ethereumNetwork:
		return app.IssuerListing{}, ""
	case n.Decimals == nil:
		return app.IssuerListing{}, "missing decimals"
	case n.Network == solanaNetwork:
		mint, err := domain.ParseMint(n.Address)
		if err != nil {
			return app.IssuerListing{}, "bad address"
		}
		l.Network, l.TokenAddress = chain.NetworkSolana, mint.String()
		return l, ""
	case n.ChainID != chain.NetworkEthereum.EVMChainID():
		return app.IssuerListing{}, "wrong chain id"
	}
	token, err := chain.ParseEVMAddress(n.Address)
	if err != nil {
		return app.IssuerListing{}, "bad address"
	}
	l.Network, l.ChainID, l.TokenAddress = chain.NetworkEthereum, n.ChainID, token.String()
	return l, ""
}
