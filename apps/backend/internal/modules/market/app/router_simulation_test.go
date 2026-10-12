package app_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const (
	delegatedCode = "0xef0100d6cedde84be40893d153be9d467cd6ad37875b28"
	otherCode     = "0xef010063c0c19a282a1b52b07dd5a65b58948a07dae32b"
)

type storedAccount struct {
	found  bool
	legacy chain.Network
	err    error
}

func (s storedAccount) Stored(context.Context) (app.SimulationAccount, bool, error) {
	a, _ := chain.ParseEVMAddress(simAddress)
	return app.SimulationAccount{WalletID: simWallet, Address: a, Legacy: s.legacy}, s.found, s.err
}

type countedCode struct {
	mu    sync.Mutex
	code  map[chain.Network]string
	err   error
	reads map[chain.Network]int
}

func (c *countedCode) Code(_ context.Context, n chain.Network, account chain.EVMAddress) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if account.String() != simAddress {
		return "", errs.New(errs.CodeInternal, "fake")
	}
	c.reads[n]++
	return c.code[n], c.err
}

func withSimulation(account storedAccount, code *countedCode) func(*routeFixture) {
	delegate, _ := chain.ParseEVMAddress(simDelegate)
	return func(f *routeFixture) { f.simulation = app.NewSimulationCheck(account, code, delegate) }
}

func codeOn(code map[chain.Network]string) *countedCode {
	return &countedCode{code: code, reads: map[chain.Network]int{}}
}

func appleWithTwoBaseListings() map[string]recordedQuote {
	quotes := recordedApple()
	quotes[aaplxBase] = recordedQuote{units: 30_000_000, gas: 76_119}
	return quotes
}

func notDelegatedOn(l map[string]multichain.Listing, names ...string) []multichain.UnavailableVenue {
	out := make([]multichain.UnavailableVenue, 0, len(names))
	for _, name := range names {
		out = append(out, multichain.UnavailableVenue{
			ListingID: l[name].ID, Network: l[name].Network, Venue: l[name].Venue,
			Reason: multichain.UnavailableSimulationFailed, Detail: app.NotDelegatedDetail,
		})
	}
	return out
}

func sameVenues(a, b []multichain.UnavailableVenue) bool {
	key := func(u multichain.UnavailableVenue) string { return u.ListingID.String() + string(u.Reason) + u.Detail }
	ka, kb := make([]string, 0, len(a)), make([]string, 0, len(b))
	for _, u := range a {
		ka = append(ka, key(u))
	}
	for _, u := range b {
		kb = append(kb, key(u))
	}
	slices.Sort(ka)
	slices.Sort(kb)
	return slices.Equal(ka, kb)
}

func TestRouterMarksAChainWithoutCodeUnavailableAndChecksEachChainOnce(t *testing.T) {
	t.Parallel()
	code := codeOn(map[chain.Network]string{chain.NetworkBase: "0x", chain.NetworkEthereum: delegatedCode})
	c, l, f, err := route(t, appleWithTwoBaseListings(), testRules(), withSimulation(storedAccount{found: true}, code))
	if err != nil || c.Winner == l[aaplcBase].ID {
		t.Fatalf("comparison = %+v, %v, want a winner off Base", c, err)
	}
	if want := notDelegatedOn(l, aaplcBase, aaplxBase); !sameVenues(c.Unavailable, want) {
		t.Fatalf("unavailable = %+v, want both Base listings not delegated", c.Unavailable)
	}
	if code.reads[chain.NetworkBase] != 1 || code.reads[chain.NetworkEthereum] != 1 || len(code.reads) != 2 {
		t.Fatalf("code reads = %v, want one per EVM chain per comparison", code.reads)
	}
	if f.bridges[chain.NetworkBase] != 0 {
		t.Fatalf("bridge estimates on Base = %d, want none for an undelegated chain", f.bridges[chain.NetworkBase])
	}
	dropped := f.logs.Lines("market.router.venue_dropped", "listing_id", "reason")
	if !slices.Contains(dropped, l[aaplcBase].ID.String()+" simulation_account_not_delegated") {
		t.Fatalf("venue_dropped = %v, want the Base listing with the reason", dropped)
	}
}

func TestRouterMarksAChainDelegatedElsewhereUnavailable(t *testing.T) {
	t.Parallel()
	code := codeOn(map[chain.Network]string{chain.NetworkBase: otherCode, chain.NetworkEthereum: delegatedCode})
	c, l, _, err := route(t, recordedApple(), testRules(), withSimulation(storedAccount{found: true}, code))
	if err != nil || c.Winner == l[aaplcBase].ID {
		t.Fatalf("comparison = %+v, %v, want a winner off Base", c, err)
	}
	if want := notDelegatedOn(l, aaplcBase); !sameVenues(c.Unavailable, want) {
		t.Fatalf("unavailable = %+v, want Base not delegated", c.Unavailable)
	}
}

func TestRouterQuotesAChainDelegatedToTheExpectedContract(t *testing.T) {
	t.Parallel()
	code := codeOn(map[chain.Network]string{chain.NetworkBase: delegatedCode, chain.NetworkEthereum: delegatedCode})
	c, l, _, err := route(t, recordedApple(), testRules(), withSimulation(storedAccount{found: true}, code))
	if err != nil || c.Winner != l[aaplcBase].ID || len(c.Unavailable) != 0 {
		t.Fatalf("comparison = %+v, %v, want Base to win as without the check", c, err)
	}
}

func TestRouterMarksEveryEVMChainUnavailableWithoutAStoredAccount(t *testing.T) {
	t.Parallel()
	code := codeOn(nil)
	c, l, _, err := route(t, recordedApple(), testRules(), withSimulation(storedAccount{}, code))
	if err != nil || len(code.reads) != 0 {
		t.Fatalf("comparison = %+v, %v, reads %v, want no code read without an account", c, err, code.reads)
	}
	if want := notDelegatedOn(l, aaplcBase, aaplonEth); !sameVenues(c.Unavailable, want) {
		t.Fatalf("unavailable = %+v, want every EVM listing not delegated", c.Unavailable)
	}
}

func TestRouterQuotesOnlyTheLegacyAccountsChain(t *testing.T) {
	t.Parallel()
	code := codeOn(map[chain.Network]string{chain.NetworkBase: delegatedCode, chain.NetworkEthereum: delegatedCode})
	account := storedAccount{found: true, legacy: chain.NetworkBase}
	c, l, _, err := route(t, recordedApple(), testRules(), withSimulation(account, code))
	if err != nil || c.Winner != l[aaplcBase].ID {
		t.Fatalf("comparison = %+v, %v, want Base to win from the legacy account as before", c, err)
	}
	if want := notDelegatedOn(l, aaplonEth); !sameVenues(c.Unavailable, want) {
		t.Fatalf("unavailable = %+v, want Ethereum not set up", c.Unavailable)
	}
	if code.reads[chain.NetworkBase] != 1 || len(code.reads) != 1 {
		t.Fatalf("code reads = %v, want Base only", code.reads)
	}
}

func TestRouterAnswersNoRouteWhenOnlyUndelegatedChainsList(t *testing.T) {
	t.Parallel()
	quotes := map[string]recordedQuote{aaplcBase: recordedApple()[aaplcBase]}
	_, _, _, err := route(t, quotes, testRules(), withSimulation(storedAccount{found: true}, codeOn(nil)))
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route so the gate falls back to the Solana mint", err)
	}
}

func TestRouterTreatsAnUnreadableCheckAsAFailedSimulation(t *testing.T) {
	t.Parallel()
	for name, tweak := range map[string]func(*routeFixture){
		"code unreadable": withSimulation(storedAccount{found: true},
			&countedCode{err: errs.New(errs.CodeRPCUnavailable, "fake"), reads: map[chain.Network]int{}}),
		"store down": withSimulation(storedAccount{err: errs.New(errs.CodeInternal, "fake")}, codeOn(nil)),
	} {
		c, l, _, err := route(t, recordedApple(), testRules(), tweak)
		want := []multichain.UnavailableVenue{
			{ListingID: l[aaplcBase].ID, Reason: multichain.UnavailableSimulationFailed},
			{ListingID: l[aaplonEth].ID, Reason: multichain.UnavailableSimulationFailed},
		}
		if err != nil || !sameVenues(c.Unavailable, want) {
			t.Fatalf("%s: comparison = %+v, %v, want the EVM listings as simulation_failed", name, c, err)
		}
	}
}
