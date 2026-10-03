package feed

import "github.com/monaco/monaco/apps/backend/internal/platform/money"

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
	VoterCount  int          `json:"voter_count,omitzero"`
	YesVotes    int          `json:"yes_votes,omitzero"`
	NoVotes     int          `json:"no_votes,omitzero"`
}
