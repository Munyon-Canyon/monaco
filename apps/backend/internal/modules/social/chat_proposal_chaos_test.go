//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	proposalChaosKnown = 8
	proposalChaosRule  = 4
)

func proposalChaosID(label string) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("chat-proposal-"+label))
}

func proposalChaosProposal(i int) uuid.UUID { return proposalChaosID("proposal" + strconv.Itoa(i)) }

func proposalChaosConsumer(t *testing.T, h testkit.Harness) bus.Consumer {
	t.Helper()
	at := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, login_provider, auth_state, auth_state_changed_at, account_status, created_at, updated_at)
		VALUES ($1, 'did:privy:chat-proposal', 'sms', 'CREATED', $2, 'active', $2, $2)`,
		proposalChaosID("proposer"), at); err != nil {
		t.Fatalf("seed the proposer: %v", err)
	}
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO cabals
		(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, created_at, updated_at)
		VALUES ($1, 'Alpha', $2, 'open', 'all', 'majority', 86400, $3, $3)`,
		proposalChaosID("cabal"), proposalChaosID("proposer"), at); err != nil {
		t.Fatalf("seed the cabal: %v", err)
	}
	heads := stubHeads{heads: map[ids.ProposalID]governanceport.ProposalHead{}}
	for i := range proposalChaosKnown {
		heads.heads[ids.ProposalIDFrom(proposalChaosProposal(i))] = governanceport.ProposalHead{
			Kind: "buy", Symbol: "NVDA", Thesis: "thesis " + strconv.Itoa(i),
		}
	}
	deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
	for _, c := range social.New(deps, social.WithProposalHeads(heads)).Consumers() {
		if c.Durable == "social_chat_proposals" {
			return c
		}
	}
	t.Fatal("social_chat_proposals is not registered")
	return bus.Consumer{}
}

func proposalChaosEvent(_ *rand.Rand, i int) events.Event {
	kind := "buy"
	if i == proposalChaosRule {
		kind = "rule"
	}
	return events.ProposalCreated{
		V: 1, ProposalID: proposalChaosProposal(i), CabalID: proposalChaosID("cabal"),
		ProposerID: proposalChaosID("proposer"), Kind: kind, Symbol: "NVDA",
		ExpiresAt: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
	}
}

func TestChatProposalConsumer_Redelivery_NoDuplicate(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return proposalChaosConsumer(t, h)
	}, proposalChaosEvent)
}
