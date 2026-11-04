package domain

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const ReasonSystemMessage = "system_message"

type ChatKind string

const (
	ChatKindUser     ChatKind = "user"
	ChatKindProposal ChatKind = "proposal"
	ChatKindSystem   ChatKind = "system"
)

type MomentKind string

const (
	MomentCabalCreated MomentKind = "cabal_created"
	MomentMemberJoined MomentKind = "member_joined"
	MomentTradeFilled  MomentKind = "trade_filled"
)

type TradeSide string

const (
	TradeBuy  TradeSide = "buy"
	TradeSell TradeSide = "sell"
)

type TradeFilled struct {
	Side        TradeSide
	Symbol      string
	AssetID     uuid.UUID
	PriceMicros int64
	USDCMicros  int64
	FilledAt    time.Time
	SwapID      uuid.UUID
	ProposalID  uuid.UUID
}

type ChatMoment struct {
	kind   MomentKind
	member ids.UserID
	trade  TradeFilled
}

type momentJSON struct {
	Kind        MomentKind `json:"kind"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	Side        TradeSide  `json:"side,omitempty"`
	Symbol      string     `json:"symbol,omitempty"`
	AssetID     *uuid.UUID `json:"asset_id,omitempty"`
	PriceMicros int64      `json:"price_micros,omitempty"`
	USDCMicros  int64      `json:"usdc_micros,omitempty"`
	FilledAt    *time.Time `json:"filled_at,omitempty"`
	SwapID      *uuid.UUID `json:"swap_id,omitempty"`
	ProposalID  *uuid.UUID `json:"proposal_id,omitempty"`
}

func NewCabalCreatedMoment() ChatMoment {
	return ChatMoment{kind: MomentCabalCreated}
}

func NewMemberJoinedMoment(user ids.UserID) (ChatMoment, error) {
	if user.UUID() == uuid.Nil {
		return ChatMoment{}, invalidMoment("social.NewMemberJoinedMoment", "no_user")
	}
	return ChatMoment{kind: MomentMemberJoined, member: user}, nil
}

func NewTradeFilledMoment(t TradeFilled) (ChatMoment, error) {
	const op = "social.NewTradeFilledMoment"
	switch {
	case t.Side != TradeBuy && t.Side != TradeSell:
		return ChatMoment{}, invalidMoment(op, "side")
	case strings.TrimSpace(t.Symbol) == "":
		return ChatMoment{}, invalidMoment(op, "symbol")
	case t.AssetID == uuid.Nil || t.SwapID == uuid.Nil:
		return ChatMoment{}, invalidMoment(op, "ids")
	case t.PriceMicros <= 0 || t.USDCMicros <= 0:
		return ChatMoment{}, invalidMoment(op, "amounts")
	case t.FilledAt.IsZero():
		return ChatMoment{}, invalidMoment(op, "filled_at")
	}
	t.FilledAt = t.FilledAt.UTC()
	return ChatMoment{kind: MomentTradeFilled, trade: t}, nil
}

func ParseChatMoment(raw []byte) (ChatMoment, error) {
	const op = "social.ParseChatMoment"
	var j momentJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return ChatMoment{}, invalidMoment(op, "malformed")
	}
	switch j.Kind {
	case MomentCabalCreated:
		return NewCabalCreatedMoment(), nil
	case MomentMemberJoined:
		if j.UserID == nil {
			return ChatMoment{}, invalidMoment(op, "no_user")
		}
		return NewMemberJoinedMoment(ids.UserIDFrom(*j.UserID))
	case MomentTradeFilled:
		if j.AssetID == nil || j.SwapID == nil || j.FilledAt == nil {
			return ChatMoment{}, invalidMoment(op, "incomplete")
		}
		t := TradeFilled{
			Side: j.Side, Symbol: j.Symbol, AssetID: *j.AssetID, PriceMicros: j.PriceMicros,
			USDCMicros: j.USDCMicros, FilledAt: *j.FilledAt, SwapID: *j.SwapID,
		}
		if j.ProposalID != nil {
			t.ProposalID = *j.ProposalID
		}
		return NewTradeFilledMoment(t)
	default:
		return ChatMoment{}, invalidMoment(op, "unknown_kind")
	}
}

func (m ChatMoment) Kind() MomentKind { return m.kind }

func (m ChatMoment) Member() ids.UserID { return m.member }

func (m ChatMoment) Trade() TradeFilled { return m.trade }

func (m ChatMoment) JSON() ([]byte, error) {
	j := momentJSON{Kind: m.kind}
	switch m.kind {
	case MomentCabalCreated:
	case MomentMemberJoined:
		id := m.member.UUID()
		j.UserID = &id
	case MomentTradeFilled:
		t := m.trade
		j.Side, j.Symbol, j.AssetID, j.PriceMicros, j.USDCMicros = t.Side, t.Symbol, &t.AssetID, t.PriceMicros, t.USDCMicros
		j.FilledAt, j.SwapID = &t.FilledAt, &t.SwapID
		if t.ProposalID != uuid.Nil {
			j.ProposalID = &t.ProposalID
		}
	default:
		return nil, invalidMoment("social.ChatMoment.JSON", "unknown_kind")
	}
	raw, _ := json.Marshal(j)
	return raw, nil
}

func invalidMoment(op, reason string) error {
	return errs.New(errs.CodeInvalidInput, op, slog.String("reason", reason))
}
