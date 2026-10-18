package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Listings interface {
	Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error)
	ListingByAsset(ctx context.Context, asset string) (multichain.Listing, error)
}

type TreasuryEVM interface {
	EnsureTreasuryEVMWallet(ctx context.Context, cabal ids.CabalID) (multichain.EVMWallet, error)
}

type UnwiredListings struct{}

func notImplemented(port, ticket string) error {
	return errs.New(
		errs.CodeNotImplemented,
		"trading.Unwired",
		slog.String("port", port),
		slog.String("ticket", ticket),
	)
}

func (UnwiredListings) Listing(context.Context, multichain.ListingID) (multichain.Listing, error) {
	return multichain.Listing{}, notImplemented("Listings.Listing", "#4969")
}

func (UnwiredListings) ListingByAsset(context.Context, string) (multichain.Listing, error) {
	return multichain.Listing{}, notImplemented("Listings.ListingByAsset", "#4969")
}

type Poll struct {
	Every time.Duration
	For   time.Duration
}

func DefaultPoll() Poll { return Poll{Every: 5 * time.Second, For: 30 * time.Second} }

func (p Poll) until(ctx context.Context, clk clock.Clock, done func() (bool, error)) error {
	deadline := clk.Now().Add(p.For)
	for {
		finished, err := done()
		if err != nil || finished || !clk.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamTimeout, "trading.Poll")
		case <-clk.After(p.Every):
		}
	}
}

type SOLTopUp interface {
	TopUpSOL(ctx context.Context, wallet chain.SolanaAddress) error
}

type Arrivals interface {
	InboundAmount(
		ctx context.Context,
		signature chain.Signature,
		owner chain.SolanaAddress,
		mint chain.Mint,
	) (money.BaseUnits, error)
}

type EVMArrival = domain.Arrival

type EVMArrivals interface {
	Inbound(
		ctx context.Context,
		network chain.Network,
		token, owner chain.EVMAddress,
		since time.Time,
		atMost money.Micros,
		exclude []string,
	) (EVMArrival, bool, error)
}

type EVMCash interface {
	Balance(ctx context.Context, network chain.Network, owner chain.EVMAddress) (money.Micros, error)
}

type RoutePorts struct {
	Listings    Listings
	TreasuryEVM TreasuryEVM
	Quoter      multichain.Quoter
	Actions     multichain.WalletActions
	Sender      EVMSender
	Venue       EVMVenue
	EVMSales    EVMSales
	Router      multichain.Router
	TopUp       SOLTopUp
	Arrivals    Arrivals
	EVMArrivals EVMArrivals
	EVM         bool
}
