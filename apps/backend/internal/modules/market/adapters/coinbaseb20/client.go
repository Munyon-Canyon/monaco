package coinbaseb20

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
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
	ContractAddress string      `json:"contract_address"`
	Symbol          string      `json:"symbol"`
	Decimals        uint8       `json:"decimals"`
	Multiplier      json.Number `json:"multiplier"`
	IsLive          bool        `json:"is_live"`
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
		l, err := toListing(t)
		if err != nil {
			return app.ListingBatch{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("symbol", t.Symbol))
		}
		batch.Listings = append(batch.Listings, l)
	}
	return batch, nil
}

func toListing(t tokenWire) (app.IssuerListing, error) {
	token, err := chain.ParseEVMAddress(t.ContractAddress)
	if err != nil {
		return app.IssuerListing{}, err
	}
	multiplier, ok := new(big.Rat).SetString(t.Multiplier.String())
	if !ok || multiplier.Sign() <= 0 {
		return app.IssuerListing{}, errs.New(errs.CodeDecodeFailed, "coinbaseb20.toListing",
			slog.String("multiplier", t.Multiplier.String()))
	}
	decimals := t.Decimals
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
	}, nil
}
