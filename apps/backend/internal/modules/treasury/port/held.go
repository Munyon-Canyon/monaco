package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HeldCabal struct {
	ID         ids.CabalID
	Name       string
	PictureURL string
}

type HeldRow struct {
	Cabal   HeldCabal
	LogoURL string
	Holding Holding
}

type HeldList struct {
	TotalValueMicros money.Micros
	CabalCount       int32
	PricesAsOf       time.Time
	Holdings         []HeldRow
}

type Held interface {
	HeldBy(ctx context.Context, viewer ids.UserID) (HeldList, error)
}
