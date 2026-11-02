package domain

import (
	"log/slog"
	"math"
	"slices"
	"strings"
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
	KindRule Kind = "rule"
)

func Kinds() []Kind { return []Kind{KindBuy, KindSell, KindRule} }

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
	TokenAmount money.Units
	Decimals    uint8
	Thesis      string
	QuoteOut    money.Units
	ExpiresAt   time.Time
	Network     chain.Network
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
	if field := tradeField(d.Kind, d.USDCMicros, d.TokenAmount, d.Thesis); field != "" {
		return field
	}
	if d.QuoteOut.IsZero() || d.Kind == KindSell && !fitsColumn(d.QuoteOut) {
		return "quote_out_amount"
	}
	return ""
}

func fitsColumn(v money.Units) bool {
	_, err := v.Int64()
	return err == nil
}

type Trade struct {
	Kind        Kind
	Symbol      string
	USDCMicros  money.Micros
	TokenAmount money.Units
	Thesis      string
	Network     chain.Network
}

func NewTrade(t Trade) (Trade, error) {
	field := tradeField(t.Kind, t.USDCMicros, t.TokenAmount, t.Thesis)
	if field == "" && strings.TrimSpace(t.Symbol) == "" {
		field = "symbol"
	}
	if field != "" {
		return Trade{}, errs.New(errs.CodeInvalidInput, "governance.NewTrade", slog.String("field", field))
	}
	return t, nil
}

func tradeField(kind Kind, usdc money.Micros, tokens money.Units, thesis string) string {
	var spendsOK bool
	switch kind {
	case KindBuy:
		spendsOK = usdc.Uint64() != 0 && usdc.Uint64() <= math.MaxInt64 && tokens.IsZero()
	case KindSell:
		spendsOK = !tokens.IsZero() && usdc.IsZero()
	case KindRule:
		fallthrough
	default:
		return "kind"
	}
	switch {
	case !spendsOK:
		return "amount"
	case !utf8.ValidString(thesis) || utf8.RuneCountInString(thesis) > MaxThesisRunes:
		return "thesis"
	}
	return ""
}
