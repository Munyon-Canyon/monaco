package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

const evmMember = chain.SolanaAddress("Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")

type memoryStore struct {
	testkit.Faults
	mu      sync.Mutex
	stored  map[ids.UserID]chain.EVMAddress
	misses  int
	members map[ids.UserID]chain.SolanaAddress
}

func (s *memoryStore) LockUser(context.Context, sqlc.DBTX, ids.UserID) error {
	return s.Check("LockUser")
}

func (s *memoryStore) StoredEVMWallet(_ context.Context, _ sqlc.DBTX, user ids.UserID) (chain.EVMAddress, bool, error) {
	if err := s.Check("StoredEVMWallet"); err != nil {
		return chain.EVMAddress{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.misses > 0 {
		s.misses--
		return chain.EVMAddress{}, false, nil
	}
	got, ok := s.stored[user]
	return got, ok, nil
}

func (s *memoryStore) MemberAddress(_ context.Context, _ sqlc.DBTX, user ids.UserID) (chain.SolanaAddress, error) {
	if err := s.Check("MemberAddress"); err != nil {
		return "", err
	}
	return s.members[user], nil
}

func (s *memoryStore) SetEVMWallet(
	_ context.Context, _ sqlc.DBTX, user ids.UserID, wallet multichain.EVMWallet, _ time.Time,
) error {
	if err := s.Check("SetEVMWallet"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[user] = wallet.Address
	return nil
}

type harness struct {
	store      *memoryStore
	wallets    *privyfake.EVMWallets
	forwarding *privyfake.Forwarding
	ensure     *app.EVMDepositWallets
	user       ids.UserID
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := testkit.DB(t)
	user := ids.NewUserID(ids.Real{})
	h := &harness{
		store: &memoryStore{
			stored: map[ids.UserID]chain.EVMAddress{}, members: map[ids.UserID]chain.SolanaAddress{user: evmMember},
		},
		wallets: &privyfake.EVMWallets{}, forwarding: &privyfake.Forwarding{}, user: user,
	}
	h.ensure = app.NewEVMDepositWallets(app.EVMDepositWalletDeps{
		UoW: db.New(pool, ids.Real{}, clock.Real{}), Reads: pool, Store: h.store,
		Wallets: h.wallets, Forwarding: h.forwarding, Clock: clock.Real{},
	})
	return h
}

func TestEnsureEVMDepositWallet_createsAttachesAndStoresThenReusesTheStoredWallet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	got, err := h.ensure.EnsureEVMDepositWallet(t.Context(), h.user)
	if err != nil || got == (chain.EVMAddress{}) || h.store.stored[h.user] != got {
		t.Fatalf("ensure = %s, %v; stored %s", got, err, h.store.stored[h.user])
	}
	again, err := h.ensure.EnsureEVMDepositWallet(t.Context(), h.user)
	if err != nil || again != got || h.wallets.Creates() != 1 || h.forwarding.Attaches() != 1 {
		t.Fatalf("second ensure = %s, %v after %d creates and %d attaches", again, err, h.wallets.Creates(),
			h.forwarding.Attaches())
	}
}

func TestEnsureEVMDepositWallet_aLoserUnderTheLockReturnsTheWinnersWallet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	won, err := h.ensure.EnsureEVMDepositWallet(t.Context(), h.user)
	if err != nil {
		t.Fatal(err)
	}
	h.store.misses = 1
	got, err := h.ensure.EnsureEVMDepositWallet(t.Context(), h.user)
	if err != nil || got != won || h.wallets.Creates() != 1 || h.forwarding.Attaches() != 1 {
		t.Fatalf("loser = %s, %v; want %s with one create and one attach", got, err, won)
	}
}

func TestEnsureEVMDepositWallet_storesNothingWhenAStepFails(t *testing.T) {
	t.Parallel()
	failure := errs.New(errs.CodePrivyUnavailable, "test")
	for _, step := range []string{"StoredEVMWallet", "LockUser", "MemberAddress", "CreateAppEVMWallet", "Attach", "SetEVMWallet"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			failsAt(t, step, failure)
		})
	}
}

func failsAt(t *testing.T, step string, failure error) {
	t.Helper()
	{
		h := newHarness(t)
		switch step {
		case "CreateAppEVMWallet":
			h.wallets.Fail(step, failure)
		case "Attach":
			h.forwarding.Fail(step, failure)
		default:
			h.store.Fail(step, failure)
		}
		if _, err := h.ensure.EnsureEVMDepositWallet(
			t.Context(),
			h.user,
		); errs.CodeOf(
			err,
		) != errs.CodePrivyUnavailable {
			t.Fatalf("%s failing: err = %v", step, err)
		}
		if len(h.store.stored) != 0 {
			t.Fatalf("%s failing: stored %v", step, h.store.stored)
		}
	}
}
