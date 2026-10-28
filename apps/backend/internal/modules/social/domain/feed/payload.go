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
	CabalName     string          `json:"cabal_name,omitempty"`
	ActorName     string          `json:"actor_name,omitempty"`
	ProposerName  string          `json:"proposer_name,omitempty"`
	Symbol        string          `json:"symbol,omitempty"`
	AssetName     string          `json:"asset_name,omitempty"`
	Action        Action          `json:"action,omitempty"`
	USDCMicros    money.Micros    `json:"usdc_micros,omitzero"`
	PriceMicros   money.Micros    `json:"price_micros,omitzero"`
	QuoteMicros   money.Micros    `json:"quote_micros,omitzero"`
	ThresholdBps  int64           `json:"threshold_bps,omitzero"`
	ShareOfPotBps int64           `json:"share_of_pot_bps,omitzero"`
	ChangeBps     int64           `json:"change_bps,omitzero"`
	MarkMicros    money.Micros    `json:"mark_micros,omitzero"`
	PrevClose     money.Micros    `json:"prev_close_micros,omitzero"`
	TradingDay    string          `json:"trading_day,omitempty"`
	TokenAmount   uint64          `json:"token_amount,string,omitzero"`
	TokenDecimals uint8           `json:"token_decimals,omitzero"`
	ProposalID    uuid.UUID       `json:"proposal_id,omitzero"`
	Status        string          `json:"status,omitempty"`
	StatusCode    string          `json:"status_code,omitempty"`
	ExpiresAt     time.Time       `json:"expires_at,omitzero"`
	Outcome       Outcome         `json:"outcome,omitempty"`
	Yes           int             `json:"yes,omitzero"`
	No            int             `json:"no,omitzero"`
	Voters        int             `json:"voters,omitzero"`
	Needed        int             `json:"needed,omitzero"`
	Shape         Shape           `json:"shape,omitempty"`
	CallerID      uuid.UUID       `json:"caller_id,omitzero"`
	CallerName    string          `json:"caller_name,omitempty"`
	AssetKind     AssetKind       `json:"asset_kind,omitempty"`
	Fills         json.RawMessage `json:"fills,omitempty"`
	Failures      json.RawMessage `json:"failures,omitempty"`
	ExpectedFills int             `json:"expected_fills,omitzero"`
	FillState     string          `json:"fill_state,omitempty"`
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
