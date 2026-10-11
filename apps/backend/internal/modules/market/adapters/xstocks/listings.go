package xstocks

import (
	"context"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
			l, reason := toListing(n, d, network)
			switch {
			case reason == "":
				batch.Listings = append(batch.Listings, l)
			case !n.IsTradingHalted:
				observability.Degraded(ctx, observability.MarketListingAssetSkipped,
					slog.String("issuer", batch.Issuer), slog.String("symbol", n.Symbol), slog.String("reason", reason))
			}
		}
		return nil
	})
	if err != nil {
		return app.ListingBatch{}, err
	}
	return batch, nil
}

func toListing(n nodeWire, d deploymentWire, network chain.Network) (app.IssuerListing, string) {
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
			return app.IssuerListing{}, "bad address"
		}
		decimals := uint8(solanaDecimals)
		l.TokenAddress, l.Decimals = mint.String(), &decimals
		return l, ""
	}
	token, err := chain.ParseEVMAddress(d.Address)
	if err != nil {
		return app.IssuerListing{}, "bad address"
	}
	l.TokenAddress = token.String()
	if d.WrapperAddressV2 != "" {
		wrapper, err := chain.ParseEVMAddress(d.WrapperAddressV2)
		if err != nil {
			return app.IssuerListing{}, "bad wrapper address"
		}
		l.WrapperAddress = wrapper.String()
	}
	return l, ""
}
