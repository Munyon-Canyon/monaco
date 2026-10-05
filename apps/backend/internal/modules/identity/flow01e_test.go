package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type stakedEveryone struct{ *fakes.Treasury }

func (s stakedEveryone) StakesOf(ctx context.Context, user ids.UserID) ([]treasury.Stake, error) {
	cabal, err := ids.ParseCabalID("01890a5d-ac96-774b-bcce-b302099a8642")
	if err != nil {
		return nil, err
	}
	s.SetStake(treasury.Stake{CabalID: cabal, UserID: user, ShareUnits: money.SharesUnitsFromUint64(1)})
	return s.Treasury.StakesOf(ctx, user)
}

type fundedEveryone struct{ *fakes.Balances }

func (b fundedEveryone) Available(ctx context.Context, user ids.UserID) (funding.Balance, error) {
	b.Set(user, funding.Balance{AvailableMicros: money.MicrosFromUint64(1)})
	return b.Balances.Available(ctx, user)
}

func deleteScenario(
	t *testing.T,
	stakes app.Stakes,
	balances app.Balances,
	extra ...scenario.Option,
) *scenario.Scenario {
	t.Helper()
	client, _, upstreams := overPrivyFakes(t)
	users, wallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config.Identity.PhotoPurgesInterval = time.Second
			return identity.New(d,
				identity.WithPrivy(users, wallets), identity.WithHoldings(stakes, balances),
				identity.WithPhotoStore(&photoStore{}),
			)
		}),
		scenario.WithPrivy(upstreams, privyAppID),
	}, extra...)...)
}

func TestFlow01e_DeleteAccount_OK(t *testing.T) {
	t.Parallel()
	s := deleteScenario(t, fakes.NewTreasury(), fakes.NewBalances())
	flows.F01eDeleteAccountOK(s)
	var status, handle string
	var deleted, pii bool
	if err := s.DB().QueryRow(t.Context(), `SELECT account_status, handle, deleted_at IS NOT NULL,
		email IS NOT NULL OR phone_e164 IS NOT NULL OR phone_hash IS NOT NULL OR phone_verified_at IS NOT NULL
		OR x_user_id IS NOT NULL OR x_username IS NOT NULL OR x_linked_at IS NOT NULL OR photo_url IS NOT NULL
		OR display_name <> ''
		FROM users WHERE privy_user_id = 'did:privy:qa-del-ok'`).Scan(&status, &handle, &deleted, &pii); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || handle != "del_gone" || !deleted || pii {
		t.Fatalf("deleted row = %s %s deleted_at set %t PII left %t, want deleted, del_gone, true, false",
			status, handle, deleted, pii)
	}
}

func TestFlow01e_DeleteAccount_AccountHasPositions(t *testing.T) {
	t.Parallel()
	flows.F01eDeleteAccountAccountHasPositions(
		deleteScenario(t, stakedEveryone{fakes.NewTreasury()}, fakes.NewBalances()),
	)
}

func TestFlow01e_DeleteAccount_AccountHasBalance(t *testing.T) {
	t.Parallel()
	flows.F01eDeleteAccountAccountHasBalance(
		deleteScenario(t, fakes.NewTreasury(), fundedEveryone{fakes.NewBalances()}),
	)
}
