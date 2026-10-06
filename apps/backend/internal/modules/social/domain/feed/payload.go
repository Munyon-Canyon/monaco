package feed

import (
	"encoding/json"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

type Payload struct {
	CabalName   string       `json:"cabal_name,omitempty"`
	ActorName   string       `json:"actor_name,omitempty"`
	Symbol      string       `json:"symbol,omitempty"`
	AssetName   string       `json:"asset_name,omitempty"`
	Action      Action       `json:"action,omitempty"`
	USDCMicros  money.Micros `json:"usdc_micros,omitzero"`
	PriceMicros money.Micros `json:"price_micros,omitzero"`
	ChangeBps   int64        `json:"change_bps,omitzero"`
	TokenAmount uint64       `json:"token_amount,string,omitzero"`
	Status      string       `json:"status,omitempty"`
	StatusCode  string       `json:"status_code,omitempty"`
	ExpiresAt   time.Time    `json:"expires_at,omitzero"`
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
