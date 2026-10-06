package feed

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

type Payload struct {
	CabalName     string       `json:"cabal_name,omitempty"`
	ActorName     string       `json:"actor_name,omitempty"`
	Symbol        string       `json:"symbol,omitempty"`
	AssetName     string       `json:"asset_name,omitempty"`
	Action        Action       `json:"action,omitempty"`
	USDCMicros    money.Micros `json:"usdc_micros,omitzero"`
	PriceMicros   money.Micros `json:"price_micros,omitzero"`
	ChangeBps     int64        `json:"change_bps,omitzero"`
	TokenAmount   uint64       `json:"token_amount,string,omitzero"`
	TokenDecimals uint8        `json:"token_decimals,omitzero"`
	ProposalID    uuid.UUID    `json:"proposal_id,omitzero"`
	Status        string       `json:"status,omitempty"`
	StatusCode    string       `json:"status_code,omitempty"`
	ExpiresAt     time.Time    `json:"expires_at,omitzero"`
}

func (p Payload) JSON() []byte {
	raw, _ := json.Marshal(p)
	return raw
}

func ParsePayload(raw []byte) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Payload{}, errs.Wrap(err, errs.CodeInternal, "feed.ParsePayload")
	}
	return p, nil
}

const maxPow10Decimals = 19

func FillPrice(usdc money.Micros, tokenAmount uint64, decimals uint8) money.Micros {
	if tokenAmount == 0 || decimals > maxPow10Decimals {
		return money.Micros{}
	}
	unit := uint64(1)
	for range decimals {
		unit *= 10
	}
	price, err := money.MulDiv(usdc.Uint64(), unit, tokenAmount)
	if err != nil {
		return money.Micros{}
	}
	return money.MicrosFromUint64(price)
}
