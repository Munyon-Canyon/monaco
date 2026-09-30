package domain

import (
	"log/slog"
	"math"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const MaxThesisRunes = 280

type Kind string

const (
	KindBuy  Kind = "buy"
	KindSell Kind = "sell"
)

func Kinds() []Kind { return []Kind{KindBuy, KindSell} }

func ParseKind(raw string) (Kind, error) {
	if !slices.Contains(Kinds(), Kind(raw)) {
		return "", unknown("governance.ParseKind", raw)
	}
	return Kind(raw), nil
}

type Draft struct {
	ID          ids.ProposalID
	CabalID     ids.CabalID
	ProposerID  ids.UserID
	Kind        Kind
	Symbol      string
	Mint        chain.SolanaAddress
	USDCMicros  money.Micros
	TokenAmount money.BaseUnits
	Thesis      string
	QuoteOut    uint64
	ExpiresAt   time.Time
}

type Proposal struct {
	draft Draft
}

func NewProposal(d Draft) (Proposal, error) {
	if field := invalidField(d); field != "" {
		return Proposal{}, errs.New(errs.CodeInvalidInput, "governance.NewProposal", slog.String("field", field))
	}
	return Proposal{draft: d}, nil
}

func (p Proposal) Draft() Draft { return p.draft }

func invalidField(d Draft) string {
	var spends, unused uint64
	switch d.Kind {
	case KindBuy:
		spends, unused = d.USDCMicros.Uint64(), d.TokenAmount.Uint64()
	case KindSell:
		spends, unused = d.TokenAmount.Uint64(), d.USDCMicros.Uint64()
	default:
		return "kind"
	}
	switch {
	case spends == 0 || spends > math.MaxInt64 || unused != 0:
		return "amount"
	case d.QuoteOut == 0 || d.QuoteOut > math.MaxInt64:
		return "quote_out_amount"
	case !utf8.ValidString(d.Thesis) || utf8.RuneCountInString(d.Thesis) > MaxThesisRunes:
		return "thesis"
	}
	return ""
}
