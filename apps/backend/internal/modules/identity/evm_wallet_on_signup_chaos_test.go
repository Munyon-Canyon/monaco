//go:build faultpoints

package identity_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func chaosSignup(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("evm-wallet-chaos-user-"+strconv.Itoa(i)))
}

func seedChaosSignup(t *testing.T, h testkit.Harness, i int, deleted, withWallet bool, at time.Time) {
	t.Helper()
	status := "active"
	if deleted {
		status = "deleted"
	}
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, 'sms', $3, $4, $3, $3, CASE WHEN $4 = 'deleted' THEN $3::timestamptz END)`,
		chaosSignup(i), "did:privy:evm-chaos-"+strconv.Itoa(i), at, status); err != nil {
		t.Fatalf("seed user %d: %v", i, err)
	}
	if !withWallet {
		return
	}
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`, chaosSignup(i), "wallet-evm-chaos-"+strconv.Itoa(i),
		"EvmChaosMember"+strconv.Itoa(i), at); err != nil {
		t.Fatalf("seed wallet %d: %v", i, err)
	}
}

func TestEVMWalletOnSignup_convergesUnderChaosWithADeletedUserAndOneWithoutAWallet(t *testing.T) {
	t.Parallel()
	const users, deleted, walletless = 4, 2, 3
	seeded := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for i := range users {
			seedChaosSignup(t, h, i, i == deleted, i != walletless, seeded)
		}
		uow := db.New(h.Pool, h.IDs, h.Clock)
		wallets := app.NewEVMDepositWallets(app.EVMDepositWalletDeps{
			UoW: uow, Reads: h.Pool, Store: adapters.EVMWallets{}, Wallets: &privyfake.EVMWallets{},
			Forwarding: &privyfake.Forwarding{}, Clock: h.Clock,
		})
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus, UoW: uow}
		deps.Config.Funding.EVMDepositNetworks = []chain.Network{chain.NetworkBase}
		for _, c := range identity.New(deps, identity.WithDepositWallets(wallets)).Consumers() {
			if c.Durable == "identity_evm_wallet" {
				return c
			}
		}
		t.Fatal("identity_evm_wallet is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		return events.UserCreated{V: 1, UserID: chaosSignup(i % users), LoginProvider: "sms", CreatedAt: seeded}
	})
}
