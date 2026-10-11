package app

import (
	"context"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type IssuerListing struct {
	Issuer                string
	Ticker                string
	Symbol                string
	Network               chain.Network
	ChainID               int64
	TokenAddress          string
	WrapperAddress        string
	Decimals              *uint8
	SharesPerToken        *big.Rat
	PendingSharesPerToken *big.Rat
	PendingAt             time.Time
	Tradable              bool
}

type ListingBatch struct {
	Issuer   string
	Listings []IssuerListing
	Missing  []string
}

type ListingSource interface {
	Issuer() string
	Listings(ctx context.Context) (ListingBatch, error)
}
