package coinbaseb20

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	maxDecimals = 36
	Issuer      = "coinbase"
	stocksPath  = "/v1/tokenized-stocks"
	maxBodySize = 4 << 20
)

var _ app.ListingSource = (*Client)(nil)

type Client struct {
	http *httpclient.Client
}

func New(c *httpclient.Client) *Client { return &Client{http: c} }

func (*Client) Issuer() string { return Issuer }

type stocksWire struct {
	Tokens []tokenWire `json:"tokens"`
}

type tokenWire struct {
	ContractAddress string       `json:"contract_address"`
	Symbol          string       `json:"symbol"`
	Decimals        *json.Number `json:"decimals"`
	Multiplier      json.Number  `json:"multiplier"`
	IsLive          bool         `json:"is_live"`
}

func (c *Client) Listings(ctx context.Context) (app.ListingBatch, error) {
	const op = "coinbaseb20.Client.Listings"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, stocksPath, nil)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return app.ListingBatch{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return app.ListingBatch{}, errs.New(errs.CodeUpstreamUnavailable, op, slog.Int("status", resp.StatusCode))
	}
	var w stocksWire
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&w); err != nil {
		return app.ListingBatch{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	batch := app.ListingBatch{Issuer: Issuer, Listings: make([]app.IssuerListing, 0, len(w.Tokens))}
	for _, t := range w.Tokens {
		l, reason := toListing(t)
		switch {
		case reason == "":
			batch.Listings = append(batch.Listings, l)
		case t.IsLive:
			observability.Degraded(ctx, observability.MarketListingAssetSkipped, slog.String("issuer", Issuer),
				slog.String("symbol", t.Symbol), slog.String("reason", reason))
		}
	}
	return batch, nil
}

func toListing(t tokenWire) (app.IssuerListing, string) {
	token, err := chain.ParseEVMAddress(t.ContractAddress)
	if err != nil {
		return app.IssuerListing{}, "bad address"
	}
	multiplier, ok := new(big.Rat).SetString(t.Multiplier.String())
	if !ok || multiplier.Sign() <= 0 {
		return app.IssuerListing{}, "bad multiplier"
	}
	if t.Decimals == nil {
		return app.IssuerListing{}, "missing decimals"
	}
	raw, err := strconv.ParseUint(t.Decimals.String(), 10, 8)
	if err != nil || raw > maxDecimals {
		return app.IssuerListing{}, "bad decimals"
	}
	decimals := uint8(raw)
	return app.IssuerListing{
		Issuer:         Issuer,
		Ticker:         strings.TrimSuffix(t.Symbol, "c"),
		Symbol:         t.Symbol,
		Network:        chain.NetworkBase,
		ChainID:        chain.NetworkBase.EVMChainID(),
		TokenAddress:   token.String(),
		Decimals:       &decimals,
		SharesPerToken: multiplier,
		Tradable:       t.IsLive,
	}, ""
}
