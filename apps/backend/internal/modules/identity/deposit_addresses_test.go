package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type evmDepositWallets struct {
	address chain.EVMAddress
	err     error
}

func (w evmDepositWallets) EnsureEVMDepositWallet(context.Context, ids.UserID) (chain.EVMAddress, error) {
	return w.address, w.err
}

func depositScenario(t *testing.T, networks []chain.Network, opts ...identity.Option) *scenario.Scenario {
	t.Helper()
	client, _, upstreams := overPrivyFakes(t)
	users, privyWallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	return scenario.New(t,
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config.Identity.NudgesInterval = time.Second
			d.Config.Funding.EVMDepositNetworks = networks
			return identity.New(d, append([]identity.Option{identity.WithPrivy(users, privyWallets)}, opts...)...)
		}),
		scenario.WithPrivy(upstreams, privyAppID),
	)
}

func TestDepositAddresses_SolanaFirstThenOneSharedEVMAddressPerNetworkInConfigOrder(t *testing.T) {
	t.Parallel()
	evm, err := chain.ParseEVMAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
	if err != nil {
		t.Fatal(err)
	}
	s := depositScenario(t, []chain.Network{chain.NetworkEthereum, chain.NetworkBase},
		identity.WithDepositWallets(evmDepositWallets{address: evm}))
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(
			scenario.ExpectStatus(200),
			scenario.ExpectJSON("addresses", []map[string]string{
				{"network": "solana", "address": string(user.Address), "asset": "USDC"},
				{"network": "ethereum", "address": evm.String(), "asset": "USDC"},
				{"network": "base", "address": evm.String(), "asset": "USDC"},
			}),
			scenario.ExpectLogs("identity.deposit_addresses.evm_failed", 0),
		)
}

func TestDepositAddresses_EVMFailureStillReturnsTheSolanaAddress(t *testing.T) {
	t.Parallel()
	failing := evmDepositWallets{err: errs.New(errs.CodePrivyUnavailable, "test.EnsureEVMDepositWallet")}
	s := depositScenario(t, []chain.Network{chain.NetworkBase}, identity.WithDepositWallets(failing))
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(
			scenario.ExpectStatus(200),
			scenario.ExpectJSON("addresses", []map[string]string{
				{"network": "solana", "address": string(user.Address), "asset": "USDC"},
			}),
			scenario.ExpectLogs("identity.deposit_addresses.evm_failed", 1),
		)
}

func TestDepositAddresses_TheEVMWalletWaitsOnTheMultichainClient(t *testing.T) {
	t.Parallel()
	s := depositScenario(t, []chain.Network{chain.NetworkBase})
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(
			scenario.ExpectStatus(200),
			scenario.ExpectJSON("addresses", []map[string]string{
				{"network": "solana", "address": string(user.Address), "asset": "USDC"},
			}),
			scenario.ExpectLogs("identity.deposit_addresses.evm_failed", 1),
		)
}

type memberWallets struct {
	wallet port.MemberWallet
	err    error
}

func (m memberWallets) MemberWallet(context.Context, ids.UserID) (port.MemberWallet, error) {
	return m.wallet, m.err
}

func TestDepositAddresses_handlerRefusesMissingCallerAndMissingWallet(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Deposits: adapters.DepositAddresses{
		Members: memberWallets{err: errs.New(errs.CodeUserNotFound, "test.MemberWallet")},
		EVM:     evmDepositWallets{err: errs.New(errs.CodeInternal, "test.EnsureEVMDepositWallet")},
	}}
	req := api.GetMyDepositAddressesRequestObject{}
	if _, err := h.GetMyDepositAddresses(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("without actor err = %v, want %s", err, errs.CodeUnauthorized)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(7).NewV7().String()})
	if _, err := h.GetMyDepositAddresses(ctx, req); errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("without wallet err = %v, want %s", err, errs.CodeUserNotFound)
	}
}

func TestDepositAddresses_noEnabledEVMNetworkSkipsTheEVMWallet(t *testing.T) {
	t.Parallel()
	address := chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
	h := adapters.HTTP{Deposits: adapters.DepositAddresses{
		Members: memberWallets{wallet: port.MemberWallet{Address: address}},
		EVM:     evmDepositWallets{err: errs.New(errs.CodeInternal, "test.EnsureEVMDepositWallet")},
	}}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(8).NewV7().String()})
	got, err := h.GetMyDepositAddresses(ctx, api.GetMyDepositAddressesRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	body, ok := got.(api.GetMyDepositAddresses200JSONResponse)
	if !ok || len(body.Addresses) != 1 || body.Addresses[0].Address != string(address) {
		t.Fatalf("response = %#v, want only the Solana address", got)
	}
}
