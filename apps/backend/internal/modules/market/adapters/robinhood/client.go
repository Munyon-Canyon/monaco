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
			l, err := toListing(a, d)
			if err != nil {
				return app.ListingBatch{}, errs.Wrap(err, errs.CodeDecodeFailed, op,
					slog.String("symbol", a.TokenSymbol))
			}
			batch.Listings = append(batch.Listings, l)
		}
	}
	return batch, nil
}

func toListing(a assetWire, d deploymentWire) (app.IssuerListing, error) {
	token, err := chain.ParseEVMAddress(d.ContractAddress)
	if err != nil {
		return app.IssuerListing{}, err
	}
	current, err := multiplier(a.CurrentMultiplier)
	if err != nil {
		return app.IssuerListing{}, err
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
		if l.PendingSharesPerToken, err = multiplier(a.PendingMultiplier); err != nil {
			return app.IssuerListing{}, err
		}
	}
	return l, nil
}

func multiplier(raw string) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(raw)
	if !ok || r.Sign() <= 0 {
		return nil, errs.New(errs.CodeDecodeFailed, "robinhood.multiplier", slog.String("multiplier", raw))
	}
	return r, nil
}
