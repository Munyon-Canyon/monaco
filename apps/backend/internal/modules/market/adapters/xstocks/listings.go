package xstocks

import (
	"context"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func listedNetwork(raw string) (chain.Network, bool) {
	switch raw {
	case solanaNetwork:
		return chain.NetworkSolana, true
	case "Ethereum":
		return chain.NetworkEthereum, true
	case "Arbitrum":
		return chain.NetworkArbitrum, true
	}
	return "", false
}

var _ app.ListingSource = ListingFeed{}

type ListingFeed struct{ *Client }

func (ListingFeed) Issuer() string { return string(domain.IssuerXStocks) }

func (c *Client) Listings(ctx context.Context) (app.ListingBatch, error) {
	batch := app.ListingBatch{Issuer: string(domain.IssuerXStocks)}
	err := c.eachNode(ctx, "xstocks.Client.Listings", func(n nodeWire) error {
		for _, d := range n.Deployments {
			network, listed := listedNetwork(d.Network)
			if !listed {
				continue
			}
			l, err := toListing(n, d, network)
			if err != nil {
				return errs.Wrap(err, errs.CodeDecodeFailed, "xstocks.Client.Listings",
					slog.String("symbol", n.Symbol), slog.String("network", d.Network))
			}
			batch.Listings = append(batch.Listings, l)
		}
		return nil
	})
	if err != nil {
		return app.ListingBatch{}, err
	}
	return batch, nil
}

func toListing(n nodeWire, d deploymentWire, network chain.Network) (app.IssuerListing, error) {
	l := app.IssuerListing{
		Issuer:   string(domain.IssuerXStocks),
		Ticker:   strings.TrimSuffix(n.Symbol, "x"),
		Symbol:   n.Symbol,
		Network:  network,
		ChainID:  network.EVMChainID(),
		Tradable: !n.IsTradingHalted,
	}
	if network == chain.NetworkSolana {
		mint, err := domain.ParseMint(d.Address)
		if err != nil {
			return app.IssuerListing{}, err
		}
		decimals := uint8(solanaDecimals)
		l.TokenAddress, l.Decimals = mint.String(), &decimals
		return l, nil
	}
	token, err := chain.ParseEVMAddress(d.Address)
	if err != nil {
		return app.IssuerListing{}, err
	}
	l.TokenAddress = token.String()
	if d.WrapperAddressV2 != "" {
		wrapper, err := chain.ParseEVMAddress(d.WrapperAddressV2)
		if err != nil {
			return app.IssuerListing{}, err
		}
		l.WrapperAddress = wrapper.String()
	}
	return l, nil
}
