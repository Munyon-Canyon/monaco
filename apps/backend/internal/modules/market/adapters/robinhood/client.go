package robinhood

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	Issuer       = "robinhood"
	assetsPath   = "/rhj/assets"
	activeStatus = "ASSET_STATUS_ACTIVE"
	maxBodySize  = 8 << 20
)

var _ app.ListingSource = (*Client)(nil)

type Client struct {
	http *httpclient.Client
}

func New(c *httpclient.Client) *Client { return &Client{http: c} }

func (*Client) Issuer() string { return Issuer }

type assetsWire struct {
	Assets []assetWire `json:"assets"`
}

type assetWire struct {
	TokenSymbol       string           `json:"tokenSymbol"`
	Deployments       []deploymentWire `json:"deployments"`
	CurrentMultiplier string           `json:"currentMultiplier"`
	PendingMultiplier string           `json:"pendingMultiplier"`
	Status            string           `json:"status"`
	TokenDecimals     *uint8           `json:"tokenDecimals"`
}

type deploymentWire struct {
	ContractAddress string `json:"contractAddress"`
	ChainID         int64  `json:"chainId"`
}

func (c *Client) Listings(ctx context.Context) (app.ListingBatch, error) {
	const op = "robinhood.Client.Listings"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, assetsPath, nil)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return app.ListingBatch{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return app.ListingBatch{}, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	var w assetsWire
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&w); err != nil {
		return app.ListingBatch{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	batch := app.ListingBatch{Issuer: Issuer}
	for _, a := range w.Assets {
		for _, d := range a.Deployments {
			if d.ChainID != chain.NetworkRobinhood.EVMChainID() {
				continue
			}
			l, reason := toListing(a, d)
			switch {
			case reason == "":
				batch.Listings = append(batch.Listings, l)
			case a.Status == activeStatus:
				observability.Degraded(ctx, observability.MarketListingAssetSkipped, slog.String("issuer", Issuer),
					slog.String("symbol", a.TokenSymbol), slog.String("reason", reason))
			}
		}
	}
	return batch, nil
}

func toListing(a assetWire, d deploymentWire) (app.IssuerListing, string) {
	token, err := chain.ParseEVMAddress(d.ContractAddress)
	if err != nil {
		return app.IssuerListing{}, "bad address"
	}
	current, ok := multiplier(a.CurrentMultiplier)
	if !ok {
		return app.IssuerListing{}, "bad multiplier"
	}
	l := app.IssuerListing{
		Issuer:         Issuer,
		Ticker:         a.TokenSymbol,
		Symbol:         a.TokenSymbol,
		Network:        chain.NetworkRobinhood,
		ChainID:        d.ChainID,
		TokenAddress:   token.String(),
		Decimals:       a.TokenDecimals,
		SharesPerToken: current,
		Tradable:       a.Status == activeStatus,
	}
	if a.PendingMultiplier != "" {
		if l.PendingSharesPerToken, ok = multiplier(a.PendingMultiplier); !ok {
			return app.IssuerListing{}, "bad pending multiplier"
		}
	}
	return l, ""
}

func multiplier(raw string) (*big.Rat, bool) {
	r, ok := new(big.Rat).SetString(raw)
	return r, ok && r.Sign() > 0
}
