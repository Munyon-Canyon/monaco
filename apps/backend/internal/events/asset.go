package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypeAssetPriceMoved Type = "asset.price_moved"

const assetAggregate = "asset"

type AssetPriceMoved struct {
	V               int          `json:"v"`
	AssetID         uuid.UUID    `json:"asset_id"`
	Symbol          string       `json:"symbol"`
	AssetName       string       `json:"asset_name"`
	ThresholdBps    int64        `json:"threshold_bps"`
	ChangeBps       int64        `json:"change_bps"`
	MarkMicros      money.Micros `json:"mark_micros"`
	PrevCloseMicros money.Micros `json:"prev_close_micros"`
	TradingDay      string       `json:"trading_day"`
	ObservedAt      time.Time    `json:"observed_at"`
}

func (AssetPriceMoved) Type() Type { return TypeAssetPriceMoved }

func (AssetPriceMoved) AggregateType() string { return assetAggregate }

func (e AssetPriceMoved) AggregateID() uuid.UUID { return e.AssetID }
