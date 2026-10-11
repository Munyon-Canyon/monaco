package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	identityapp "github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
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

func TestDepositAddresses_DefaultWiringFallsBackToSolanaWhenPrivyFails(t *testing.T) {
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

type depositFees struct {
	fees  map[chain.Network]money.Micros
	err   error
	calls *int
}

func (f depositFees) EstimatedDepositFees(context.Context) (map[chain.Network]money.Micros, error) {
	*f.calls++
	return f.fees, f.err
}

func testEVMAddress(t *testing.T) chain.EVMAddress {
	t.Helper()
	evm, err := chain.ParseEVMAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
	if err != nil {
		t.Fatal(err)
	}
	return evm
}

func TestDepositAddresses_EVMItemsCarryTheLastForwardFeeOfTheirNetwork(t *testing.T) {
	t.Parallel()
	evm, calls := testEVMAddress(t), 0
	fees := depositFees{fees: map[chain.Network]money.Micros{
		chain.NetworkBase: money.MicrosFromUint64(163_098), chain.NetworkSolana: money.MicrosFromUint64(1),
	}, calls: &calls}
	s := depositScenario(t, []chain.Network{chain.NetworkBase, chain.NetworkArbitrum},
		identity.WithDepositWallets(evmDepositWallets{address: evm}), identity.WithDepositFees(fees))
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(
			scenario.ExpectStatus(200),
			scenario.ExpectJSON("addresses", []map[string]string{
				{"network": "solana", "address": string(user.Address), "asset": "USDC"},
				{"network": "base", "address": evm.String(), "asset": "USDC", "estimated_fee_micros": "163098"},
				{"network": "arbitrum", "address": evm.String(), "asset": "USDC"},
			}),
			scenario.ExpectLogs("identity.deposit_addresses.fee_failed", 0),
		)
	if calls != 1 {
		t.Fatalf("fee reads = %d, want one per request", calls)
	}
}

func TestDepositAddresses_FeeReadFailureStillReturnsEveryAddress(t *testing.T) {
	t.Parallel()
	evm, calls := testEVMAddress(t), 0
	fees := depositFees{err: errs.New(errs.CodeInternal, "test.EstimatedDepositFees"), calls: &calls}
	s := depositScenario(t, []chain.Network{chain.NetworkBase},
		identity.WithDepositWallets(evmDepositWallets{address: evm}), identity.WithDepositFees(fees))
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/deposit-addresses")).
		Then(
			scenario.ExpectStatus(200),
			scenario.ExpectJSON("addresses", []map[string]string{
				{"network": "solana", "address": string(user.Address), "asset": "USDC"},
				{"network": "base", "address": evm.String(), "asset": "USDC"},
			}),
			scenario.ExpectLogs("identity.deposit_addresses.fee_failed", 1),
		)
}

type feeProvider struct{ depositFees }

func (feeProvider) Name() string              { return "fee-provider" }
func (feeProvider) Mount(baseapi.Mount)       {}
func (feeProvider) Consumers() []bus.Consumer { return nil }
func (feeProvider) Pollers() []poller.Poller  { return nil }

func TestDepositAddresses_WiresTheFeeFromTheModuleThatEstimatesIt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		option bool
		want   []map[string]string
	}{
		"wired module": {want: []map[string]string{{"network": "base", "estimated_fee_micros": "163867"}}},
		"option wins":  {option: true, want: []map[string]string{{"network": "base"}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			evm, wired, preset := testEVMAddress(t), 0, 0
			provider := feeProvider{depositFees{
				fees: map[chain.Network]money.Micros{chain.NetworkBase: money.MicrosFromUint64(163_867)}, calls: &wired,
			}}
			client, _, upstreams := overPrivyFakes(t)
			users, privyWallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
			opts := []identity.Option{
				identity.WithPrivy(users, privyWallets), identity.WithDepositWallets(evmDepositWallets{address: evm}),
			}
			if tc.option {
				opts = append(opts, identity.WithDepositFees(depositFees{calls: &preset}))
			}
			s := scenario.New(t,
				scenario.WithModules(func(d module.Deps) module.Module {
					d.Config.Funding.EVMDepositNetworks = []chain.Network{chain.NetworkBase}
					return identity.New(d, opts...)
				}, func(module.Deps) module.Module { return provider }),
				scenario.WithPrivy(upstreams, privyAppID),
			)
			user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
			want := append([]map[string]string{{"network": "solana", "address": string(user.Address), "asset": "USDC"}},
				tc.want...)
			want[1]["address"], want[1]["asset"] = evm.String(), "USDC"
			s.Given(scenario.AsSeededUser("member", user.ID)).
				When(scenario.Get("/v1/me/deposit-addresses")).
				Then(scenario.ExpectStatus(200), scenario.ExpectJSON("addresses", want))
			if tc.option && (wired != 0 || preset != 1) {
				t.Fatalf("wired reads = %d, option reads = %d; want only the option read", wired, preset)
			}
		})
	}
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

func TestEVMDepositWallets_readFailureIsInternal(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	wallets := adapters.NewEnrolledEVMWallets(testkit.DB(t))
	if _, err := wallets.EVMDepositWallets(ctx, ids.UserID{}, 10); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("EVMDepositWallets on a cancelled context = %v, want %s", err, errs.CodeInternal)
	}
}

func TestDepositAddresses_EnsuresOneForwardedEVMWalletAndServesItOnEveryNetwork(t *testing.T) {
	t.Parallel()
	wallets, forwarding := &privyfake.EVMWallets{}, &privyfake.Forwarding{}
	client, _, upstreams := overPrivyFakes(t)
	users, privyWallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	s := scenario.New(t,
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config.Funding.EVMDepositNetworks = []chain.Network{chain.NetworkBase, chain.NetworkArbitrum}
			ensure := identityapp.NewEVMDepositWallets(identityapp.EVMDepositWalletDeps{
				UoW: d.UoW, Reads: d.Pool, Store: adapters.EVMWallets{}, Wallets: wallets, Forwarding: forwarding,
				Clock: d.Clock,
			})
			return identity.New(d, identity.WithPrivy(users, privyWallets), identity.WithDepositWallets(ensure))
		}),
		scenario.WithPrivy(upstreams, privyAppID),
	)
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	created, err := wallets.CreateAppEVMWallet(t.Context(), "evm-deposit:"+user.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s.Given(scenario.AsSeededUser("member", user.ID)).
			When(scenario.Get("/v1/me/deposit-addresses")).
			Then(
				scenario.ExpectStatus(200),
				scenario.ExpectJSON("addresses", []map[string]string{
					{"network": "solana", "address": string(user.Address), "asset": "USDC"},
					{"network": "base", "address": created.Address.String(), "asset": "USDC"},
					{"network": "arbitrum", "address": created.Address.String(), "asset": "USDC"},
				}),
			)
	}
	if forwarding.Attaches() != 1 || forwarding.Destination(created.ID) != user.Address {
		t.Fatalf("attaches = %d to %s, want one attach to the member wallet %s",
			forwarding.Attaches(), forwarding.Destination(created.ID), user.Address)
	}
}
