package feed

import (
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Kind string

const (
	KindProposal     Kind = "proposal"
	KindTrade        Kind = "trade"
	KindPriceMove    Kind = "price_move"
	KindCabalCreated Kind = "cabal_created"
	KindMemberJoined Kind = "member_joined"
	KindReply        Kind = "reply"
	KindThreshold    Kind = "threshold"
	KindResult       Kind = "result"
)

func Kinds() []Kind {
	return []Kind{
		KindProposal,
		KindTrade,
		KindPriceMove,
		KindCabalCreated,
		KindMemberJoined,
		KindReply,
		KindThreshold,
		KindResult,
	}
}

func ParseKind(raw string) (Kind, error) {
	if k := Kind(raw); slices.Contains(Kinds(), k) {
		return k, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "feed.ParseKind", slog.String("kind", raw))
}

type RefType string

const (
	RefProposals       RefType = "proposals"
	RefSwaps           RefType = "swaps"
	RefAssetPriceMoves RefType = "asset_price_moves"
	RefCabals          RefType = "cabals"
	RefCabalMembers    RefType = "cabal_members"
	RefFeedComments    RefType = "feed_comments"
)

func (k Kind) RefType() RefType {
	switch k {
	case KindProposal, KindThreshold, KindResult:
		return RefProposals
	case KindTrade:
		return RefSwaps
	case KindPriceMove:
		return RefAssetPriceMoves
	case KindCabalCreated:
		return RefCabals
	case KindMemberJoined:
		return RefCabalMembers
	case KindReply:
		return RefFeedComments
	default:
		return ""
	}
}

type Tone string

const (
	ToneNeutral  Tone = "neutral"
	TonePositive Tone = "positive"
	ToneNegative Tone = "negative"
)

type Sort string

const (
	SortNew Sort = "new"
	SortTop Sort = "top"
)

const TopWindow = 24 * time.Hour
