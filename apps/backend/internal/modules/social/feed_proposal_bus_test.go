package social_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFeedProposal_anExpiryThroughTheBusEndsExpired(t *testing.T) {
	t.Parallel()
	for _, expiryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "after the open", true: "before the open"}[expiryFirst], func(t *testing.T) {
			t.Parallel()
			expiryThroughTheBus(t, expiryFirst)
		})
	}
}

func expiryThroughTheBus(t *testing.T, expiryFirst bool) {
	t.Helper()
	gen := testkit.NewIDs(testkit.RandSeed(t))
	creator, proposer, cabal, proposal := gen.NewV7(), gen.NewV7(), gen.NewV7(), gen.NewV7()
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: ids.UserIDFrom(creator), Handle: "alice"}, {ID: ids.UserIDFrom(proposer), Handle: "bob"},
	}, nil)
	var uow *db.UnitOfWork
	appendEvents := func(evs ...events.Event) scenario.Step {
		return func(s *scenario.Scenario) {
			s.Helper()
			ctx := observability.WithActor(s.Context(), "system:test")
			err := uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
				for _, e := range evs {
					if err := tx.Events.Append(ctx, e); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				s.Fatalf("append: %v", err)
			}
		}
	}
	status := func(want string) scenario.Step {
		return scenario.Eventually("feed item "+want, func(s *scenario.Scenario) bool {
			var got string
			const q = `SELECT status FROM feed_objects WHERE kind = 'proposal' AND ref_id = $1`
			err := s.DB().QueryRow(s.Context(), q, proposal).Scan(&got)
			return err == nil && got == want
		})
	}
	s := scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		uow = d.UoW
		return social.New(d, social.WithUsers(users), social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx())))
	}))
	expiresAt := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	created := events.ProposalCreated{
		V: 1, ProposalID: proposal, CabalID: cabal, ProposerID: proposer, Kind: "buy", Symbol: "AAPLx",
		Mint: marketfake.AAPLx().Mint.Address(), USDCMicros: money.MicrosFromUint64(500_000_000),
		ExpiresAt: expiresAt, VoterCount: 3,
	}
	expired := events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal}
	open := []scenario.Step{appendEvents(created), status("open"), appendEvents(expired)}
	if expiryFirst {
		open = []scenario.Step{appendEvents(expired), appendEvents(created)}
	}
	s.Given(appendEvents(events.CabalCreated{V: 1, CabalID: cabal, CreatorID: creator, Name: "Alpha"})).
		When(open...).
		Then(status("expired"))
}

type sqlSeed struct {
	gen                               *testkit.IDs
	cabal, proposer, proposal         uuid.UUID
	createdAt, expiresAt, expiresJSON string
}

func newSQLSeed(t *testing.T, closed bool) sqlSeed {
	t.Helper()
	gen := testkit.NewIDs(testkit.RandSeed(t))
	seed := sqlSeed{
		gen: gen, cabal: gen.NewV7(), proposer: gen.NewV7(), proposal: gen.NewV7(),
		createdAt: "0 seconds", expiresAt: "86400 seconds", expiresJSON: "2030-01-02T00:00:00Z",
	}
	if closed {
		seed.createdAt, seed.expiresAt, seed.expiresJSON = "-2 days", "-1 day", "2020-01-02T00:00:00Z"
	}
	return seed
}

func (q sqlSeed) event(typ, payload, actorType, actorID, at string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		const insert = `INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
			VALUES ($1, 'proposal', $2, $3, $4::jsonb, $5, $6, now() + $7::interval)`
		id := q.gen.NewV7()
		_, err := s.DB().Exec(s.Context(), insert, id, q.proposal, typ, payload, actorType, actorID, at)
		if err != nil {
			s.Fatalf("seed %s: %v", typ, err)
		}
	}
}

func (q sqlSeed) created() scenario.Step {
	payload := fmt.Sprintf(`{"v":1,"proposal_id":%q,"cabal_id":%q,"proposer_id":%q,"kind":"buy","symbol":"AAPLx",
		"mint":%q,"usdc_micros":"5000000","quote_out_amount":"12345","expires_at":%q,"voter_count":3}`,
		q.proposal, q.cabal, q.proposer, marketfake.AAPLx().Mint.Address(), q.expiresJSON)
	return q.event("proposal.created", payload, "user", q.proposer.String(), q.createdAt)
}

func (q sqlSeed) expired() scenario.Step {
	payload := fmt.Sprintf(`{"v":1,"proposal_id":%q,"cabal_id":%q}`, q.proposal, q.cabal)
	return q.event("proposal.expired", payload, "system", "poller.governance.proposal_expiry", q.expiresAt)
}

func TestFeedProposal_aSqlSeededExpiryThroughTheBusEndsExpired(t *testing.T) {
	t.Parallel()
	for _, closed := range []bool{true, false} {
		t.Run(map[bool]string{true: "past-dated closed", false: "open then expired"}[closed], func(t *testing.T) {
			t.Parallel()
			q := newSQLSeed(t, closed)
			creator := q.gen.NewV7()
			users := fakes.NewIdentity([]identity.UserCard{
				{ID: ids.UserIDFrom(creator), Handle: "alice"}, {ID: ids.UserIDFrom(q.proposer), Handle: "bob"},
			}, nil)
			var uow *db.UnitOfWork
			s := scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
				uow = d.UoW
				assets := social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx()))
				return social.New(d, social.WithUsers(users), assets)
			}))
			cabalEvent := func(s *scenario.Scenario) {
				s.Helper()
				ctx := observability.WithActor(s.Context(), "system:test")
				err := uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
					e := events.CabalCreated{V: 1, CabalID: q.cabal, CreatorID: creator, Name: "Alpha"}
					return tx.Events.Append(ctx, e)
				})
				if err != nil {
					s.Fatalf("append: %v", err)
				}
			}
			steps := []scenario.Step{q.created(), q.expired()}
			if !closed {
				steps = []scenario.Step{q.created(), feedStatus(q.proposal, "open"), q.expired()}
			}
			s.Given(cabalEvent).When(steps...).Then(feedStatus(q.proposal, "expired"))
		})
	}
}

func feedStatus(proposal uuid.UUID, want string) scenario.Step {
	return scenario.Eventually("feed item "+want, func(s *scenario.Scenario) bool {
		var got string
		const q = `SELECT status FROM feed_objects WHERE kind = 'proposal' AND ref_id = $1`
		err := s.DB().QueryRow(s.Context(), q, proposal).Scan(&got)
		return err == nil && got == want
	})
}
