//go:build faultpoints

package cabal_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func chaosTreasuryID(what string, i int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("treasury-evm-chaos-"+what+"-"+strconv.Itoa(i)))
}

func seedChaosTreasury(t *testing.T, h testkit.Harness, i int, seeded time.Time) {
	t.Helper()
	creator, id := chaosTreasuryID("user", i), chaosTreasuryID("cabal", i)
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at)
		VALUES ($1, $2, 'sms', $3, 'active', $3, $3)`,
		creator, "did:privy:treasury-evm-chaos-"+strconv.Itoa(i), seeded); err != nil {
		t.Fatalf("seed creator %d: %v", i, err)
	}
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO cabals
		(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, created_at, updated_at)
		VALUES ($1, $2, $3, 'open', 'all', 'majority', 86400, $4, $4)`,
		id, "evm chaos "+strconv.Itoa(i), creator, seeded); err != nil {
		t.Fatalf("seed cabal %d: %v", i, err)
	}
	if _, err := h.Pool.Exec(
		t.Context(),
		`INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`,
		id,
		"treasury-evm-chaos-"+strconv.Itoa(i),
		"TreasuryEvmChaos"+strconv.Itoa(i),
		seeded,
	); err != nil {
		t.Fatalf("seed treasury %d: %v", i, err)
	}
}

func TestTreasuryEVMWalletOnCreate_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	const cabals = 4
	seeded := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for i := range cabals {
			seedChaosTreasury(t, h, i, seeded)
		}
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		for _, c := range cabal.New(deps, cabal.WithEVMWallets(&privyfake.EVMWallets{})).Consumers() {
			if c.Durable == "cabal_evm_wallet" {
				return c
			}
		}
		t.Fatal("cabal_evm_wallet is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		n := i % cabals
		return events.CabalCreated{
			V: 1, CabalID: chaosTreasuryID("cabal", n), CreatorID: chaosTreasuryID("user", n),
			Name: "evm chaos " + strconv.Itoa(n), JoinMode: "open", VoterMode: "all", Threshold: "majority",
			ProposalExpirySeconds: 86400, TreasuryAddress: chain.SolanaAddress("TreasuryEvmChaos" + strconv.Itoa(n)),
		}
	})
}
