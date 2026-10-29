package app

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	marketFactsMaxSymbols = 50
	marketFactsMaxLength  = 64
	marketFactsCalledFor  = 7 * 24 * time.Hour
	marketFactsDecimals   = 4
)

type MarketFactKind string

const (
	MarketFactVoteOpen MarketFactKind = "vote_open"
	MarketFactHeld     MarketFactKind = "held"
	MarketFactCalled   MarketFactKind = "called"
)

type MarketFact struct {
	Kind         MarketFactKind
	CabalName    string
	Count        int
	Units        string
	CabalCount   int
	ProposerName string
}

type MarketFactRow struct {
	Symbol string
	Fact   *MarketFact
}

type MarketFactCabals interface {
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
	Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error)
}

type MarketFacts struct {
	q      *sqlc.Queries
	cabals MarketFactCabals
	held   treasuryport.Held
	users  Handles
	clock  clock.Clock
}

func NewMarketFacts(
	db sqlc.DBTX, cabals MarketFactCabals, held treasuryport.Held, users Handles, c clock.Clock,
) *MarketFacts {
	return &MarketFacts{q: sqlc.New(db), cabals: cabals, held: held, users: users, clock: c}
}

func ParseMarketFactSymbols(raw string) ([]string, error) {
	const op = "governance.ParseMarketFactSymbols"
	symbols := strings.Split(raw, ",")
	if len(symbols) > marketFactsMaxSymbols {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("symbols", len(symbols)))
	}
	for i, symbol := range symbols {
		symbols[i] = strings.TrimSpace(symbol)
		if symbols[i] == "" || len(symbols[i]) > marketFactsMaxLength {
			return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("symbol_index", i))
		}
	}
	return symbols, nil
}

func (r *MarketFacts) For(ctx context.Context, caller ids.UserID, symbols []string) ([]MarketFactRow, error) {
	const op = "governance.MarketFacts.For"
	active, names, err := r.activeCabals(ctx, caller)
	if err != nil {
		return nil, err
	}
	out := make([]MarketFactRow, len(symbols))
	for i, symbol := range symbols {
		out[i].Symbol = symbol
	}
	if len(active) == 0 {
		return out, nil
	}
	wanted := distinct(symbols)
	open, err := r.q.OpenFactsForVoter(ctx, sqlc.OpenFactsForVoterParams{
		Symbols: wanted, CallerID: caller.UUID(), CabalIds: uuids(active),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	called, err := r.q.CalledFacts(ctx, sqlc.CalledFactsParams{
		Symbols: wanted, Since: r.clock.Now().Add(-marketFactsCalledFor), CabalIds: uuids(active),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	held, err := r.heldBy(ctx, caller)
	if err != nil {
		return nil, err
	}
	proposers, err := r.proposerNames(ctx, called)
	if err != nil {
		return nil, err
	}
	facts := map[string]*MarketFact{}
	for _, row := range called {
		facts[row.Symbol] = &MarketFact{Kind: MarketFactCalled, ProposerName: proposers[ids.UserIDFrom(row.ProposerID)]}
	}
	for symbol, fact := range held {
		facts[symbol] = fact
	}
	for _, row := range open {
		facts[row.Symbol] = &MarketFact{
			Kind: MarketFactVoteOpen, CabalName: names[ids.CabalIDFrom(row.CabalID)], Count: int(row.OpenCount),
		}
	}
	for i := range out {
		out[i].Fact = facts[out[i].Symbol]
	}
	return out, nil
}

func (r *MarketFacts) activeCabals(
	ctx context.Context, caller ids.UserID,
) ([]ids.CabalID, map[ids.CabalID]string, error) {
	const op = "governance.MarketFacts.activeCabals"
	mine, err := r.cabals.CabalsOf(ctx, caller)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(mine) == 0 {
		return nil, nil, nil
	}
	views, err := r.cabals.Cabals(ctx, mine)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	active, names := make([]ids.CabalID, 0, len(mine)), make(map[ids.CabalID]string, len(mine))
	for _, id := range mine {
		if view, ok := views[id]; ok && view.Status == cabalport.StatusActive {
			active, names[id] = append(active, id), view.Name
		}
	}
	return active, names, nil
}

func (r *MarketFacts) heldBy(ctx context.Context, caller ids.UserID) (map[string]*MarketFact, error) {
	const op = "governance.MarketFacts.heldBy"
	list, err := r.held.HeldBy(ctx, caller)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	units, cabals := map[string]*big.Rat{}, map[string]map[ids.CabalID]bool{}
	for _, row := range list.Holdings {
		amount, ok := new(big.Rat).SetString(row.Holding.Units)
		if !ok {
			return nil, errs.New(errs.CodeInternal, op, slog.String("symbol", row.Holding.Symbol))
		}
		symbol := row.Holding.Symbol
		if units[symbol] == nil {
			units[symbol], cabals[symbol] = new(big.Rat), map[ids.CabalID]bool{}
		}
		units[symbol].Add(units[symbol], amount)
		cabals[symbol][row.Cabal.ID] = true
	}
	out := make(map[string]*MarketFact, len(units))
	for symbol, sum := range units {
		if sum.Sign() > 0 {
			out[symbol] = &MarketFact{
				Kind: MarketFactHeld, Units: sum.FloatString(marketFactsDecimals), CabalCount: len(cabals[symbol]),
			}
		}
	}
	return out, nil
}

func (r *MarketFacts) proposerNames(
	ctx context.Context, called []sqlc.CalledFactsRow,
) (map[ids.UserID]string, error) {
	if len(called) == 0 {
		return map[ids.UserID]string{}, nil
	}
	who := make([]ids.UserID, len(called))
	for i, row := range called {
		who[i] = ids.UserIDFrom(row.ProposerID)
	}
	cards, err := r.users.UsersByID(ctx, who)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "governance.MarketFacts.proposerNames")
	}
	names := make(map[ids.UserID]string, len(cards))
	for id, card := range cards {
		names[id] = nameOr(card.DisplayName, card.Handle)
	}
	return names, nil
}

func distinct(symbols []string) []string {
	seen, out := make(map[string]bool, len(symbols)), make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if !seen[symbol] {
			seen[symbol] = true
			out = append(out, symbol)
		}
	}
	return out
}

func uuids(cabals []ids.CabalID) []uuid.UUID {
	out := make([]uuid.UUID, len(cabals))
	for i, id := range cabals {
		out[i] = id.UUID()
	}
	return out
}
