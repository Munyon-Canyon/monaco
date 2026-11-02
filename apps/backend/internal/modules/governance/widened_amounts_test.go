package governance_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
)

func TestOpenProposal_roundTripsWideTokenAmountsExactlyThroughEveryRead(t *testing.T) {
	t.Parallel()
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	max256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for name, want := range map[string]string{"2^64": two64.String(), "2^256-1": max256.String()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newProposalDB(t)
			voter := d.ids.NewV7()
			p := d.buy(voter)
			p.Kind, p.UsdcMicros, p.TokenAmount = "sell", pgtype.Int8{}, want
			p.QuoteOutAmount = want
			if err := d.openWide(t, p, voter); err != nil {
				t.Fatal(err)
			}
			d.wantWide(t, p.ID, p.CabalID, voter, want, want)
		})
	}
}

func (d proposalDB) openWide(t *testing.T, p sqlc.InsertProposalParams, voter uuid.UUID) error {
	t.Helper()
	_, err := d.q.OpenProposal(t.Context(), sqlc.OpenProposalParams{
		VoterIds: []uuid.UUID{voter}, ID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID, Kind: p.Kind,
		Symbol: p.Symbol, Mint: p.Mint, UsdcMicros: "0", TokenAmount: p.TokenAmount, Thesis: p.Thesis.String,
		QuoteOutAmount: p.QuoteOutAmount, Threshold: p.Threshold, ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt,
	})
	return err
}

type wideRead struct {
	name string
	read func(ctx context.Context) ([2]string, error)
}

func (d proposalDB) wantWide(t *testing.T, id, cabal, voter uuid.UUID, tokens, quote string) {
	t.Helper()
	reads := []wideRead{
		{"GetProposal", func(ctx context.Context) ([2]string, error) {
			row, err := d.q.GetProposal(ctx, sqlc.GetProposalParams{ID: id, CallerID: voter})
			return [2]string{row.TokenAmount, row.QuoteOutAmount}, err
		}},
		{"ListProposals", func(ctx context.Context) ([2]string, error) {
			rows, err := d.q.ListProposals(ctx, sqlc.ListProposalsParams{
				CallerID: voter, CabalID: cabal, Filter: "all", RowLimit: 10,
			})
			return onlyRow(
				len(rows),
				func() [2]string { return [2]string{rows[0].TokenAmount, rows[0].QuoteOutAmount} },
				err,
			)
		}},
		{"ListMyProposals", func(ctx context.Context) ([2]string, error) {
			rows, err := d.q.ListMyProposals(
				ctx,
				sqlc.ListMyProposalsParams{CallerID: voter, Filter: "all", RowLimit: 10},
			)
			return onlyRow(
				len(rows),
				func() [2]string { return [2]string{rows[0].TokenAmount, rows[0].QuoteOutAmount} },
				err,
			)
		}},
		{"PendingVotes", func(ctx context.Context) ([2]string, error) {
			rows, err := d.q.PendingVotes(ctx, sqlc.PendingVotesParams{VoterID: voter, CabalIds: []uuid.UUID{cabal}})
			return onlyRow(len(rows), func() [2]string { return [2]string{rows[0].TokenAmount, quote} }, err)
		}},
		{"ProposalCards", func(ctx context.Context) ([2]string, error) {
			rows, err := d.q.ProposalCards(ctx, sqlc.ProposalCardsParams{ViewerID: voter, ProposalIds: []uuid.UUID{id}})
			return onlyRow(len(rows), func() [2]string { return [2]string{rows[0].TokenAmount, quote} }, err)
		}},
		{"LockProposal", func(ctx context.Context) ([2]string, error) {
			row, err := d.q.LockProposal(ctx, sqlc.LockProposalParams{ID: id, VoterID: voter})
			return [2]string{row.TokenAmount, row.QuoteOutAmount}, err
		}},
	}
	for _, r := range reads {
		got, err := r.read(t.Context())
		if err != nil || got != [2]string{tokens, quote} {
			t.Errorf("%s = %v, %v, want token_amount %s and quote_out_amount %s", r.name, got, err, tokens, quote)
		}
	}
}

func onlyRow(n int, pair func() [2]string, err error) ([2]string, error) {
	if err != nil || n != 1 {
		return [2]string{}, errors.Join(err, fmt.Errorf("%d rows, want 1", n))
	}
	return pair(), nil
}
