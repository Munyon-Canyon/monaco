package admin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func portDown() error { return errs.New(errs.CodeUpstreamUnavailable, "test.port") }

func isPortDown(err error) bool { return errs.CodeOf(err) == errs.CodeUpstreamUnavailable }

type stubPorts struct {
	g       *testkit.IDs
	fail    string
	wallet  error
	payload string
	cabal   cabalport.CabalView
}

func (s stubPorts) err(method string) error {
	if s.fail == method {
		return portDown()
	}
	return nil
}

func (s stubPorts) UsersByID(_ context.Context, userIDs []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	out := map[ids.UserID]identityport.UserCard{}
	for _, id := range userIDs {
		out[id] = identityport.UserCard{ID: id, Handle: "h"}
	}
	return out, s.err("UsersByID")
}

func (s stubPorts) UserByHandle(context.Context, string) (identityport.UserCard, error) {
	return identityport.UserCard{ID: ids.UserIDFrom(s.g.NewV7())}, s.err("UserByHandle")
}

func (s stubPorts) MemberWallet(context.Context, ids.UserID) (identityport.MemberWallet, error) {
	if s.wallet != nil {
		return identityport.MemberWallet{}, s.wallet
	}
	return identityport.MemberWallet{Address: "ABCDEFGHIJKL"}, s.err("MemberWallet")
}

func (s stubPorts) Cabal(context.Context, ids.CabalID) (cabalport.CabalView, error) {
	return s.cabal, s.err("Cabal")
}

func (s stubPorts) Cabals(_ context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	return map[ids.CabalID]cabalport.CabalView{cabalIDs[0]: {Name: "c"}}, s.err("Cabals")
}

func (s stubPorts) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return []ids.CabalID{ids.CabalIDFrom(s.g.NewV7()), ids.CabalIDFrom(s.g.NewV7())}, s.err("CabalsOf")
}

func (s stubPorts) Members(context.Context, ids.CabalID) ([]cabalport.MemberView, error) {
	return []cabalport.MemberView{{UserID: ids.UserIDFrom(s.g.NewV7())}}, s.err("Members")
}

func (s stubPorts) Rules(context.Context, ids.CabalID) (cabalport.Rules, error) {
	return cabalport.Rules{}, s.err("Rules")
}

func (s stubPorts) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{}, s.err("TreasuryWallet")
}

func (s stubPorts) UserTxns(
	context.Context, ids.UserID, *treasuryport.TxnCursor, int,
) ([]treasuryport.TxnHeader, error) {
	return nil, s.err("UserTxns")
}

func (s stubPorts) CabalTxns(
	context.Context, ids.CabalID, *treasuryport.TxnCursor, int,
) ([]treasuryport.TxnHeader, error) {
	return nil, s.err("CabalTxns")
}

func (s stubPorts) UserShares(context.Context, ids.UserID) ([]treasuryport.Share, error) {
	return nil, s.err("UserShares")
}

func (s stubPorts) CabalShares(context.Context, ids.CabalID) ([]treasuryport.Share, error) {
	return nil, s.err("CabalShares")
}

func (s stubPorts) CabalHoldings(context.Context, ids.CabalID) ([]treasuryport.RawHolding, error) {
	return nil, s.err("CabalHoldings")
}

func (s stubPorts) ListAll(context.Context) ([]market.Asset, error) { return nil, s.err("ListAll") }

func (s stubPorts) EventsByAggregate(
	context.Context, string, uuid.UUID, []string, int,
) ([]bus.EventRow, error) {
	payload := s.payload
	if payload == "" {
		payload = `{"v":1}`
	}
	return []bus.EventRow{{Payload: []byte(payload)}}, s.err("EventsByAggregate")
}

func (s stubPorts) Recent(context.Context, string, string, int) ([]sqlc.AdminAction, error) {
	return nil, s.err("Recent")
}

func newStub(opts ...func(*stubPorts)) stubPorts {
	s := stubPorts{g: testkit.NewIDs(71)}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

func failing(method string) func(*stubPorts) { return func(s *stubPorts) { s.fail = method } }

func userLookup(s stubPorts) app.UserLookup {
	return app.UserLookup{
		Users: s, Wallets: s, Cabals: s, Shares: s, Txns: s, Events: s, Actions: s,
	}
}

func cabalLookup(s stubPorts) app.CabalLookup {
	return app.CabalLookup{
		Cabals: s, Details: s, Users: s, Shares: s, Holdings: s, Assets: s, Txns: s, Actions: s,
	}
}

func TestUserLookup_PassesEveryPortFailureOn(t *testing.T) {
	t.Parallel()
	id := ids.UserIDFrom(newStub().g.NewV7())
	for _, method := range []string{
		"UsersByID", "MemberWallet", "EventsByAggregate", "CabalsOf", "Cabals", "UserShares", "UserTxns", "Recent",
	} {
		if _, err := userLookup(newStub(failing(method))).ByID(t.Context(), id); !isPortDown(err) {
			t.Errorf("ByID with %s down = %v", method, err)
		}
	}
	byHandle := userLookup(newStub(failing("UserByHandle")))
	if _, err := byHandle.ByHandle(t.Context(), "h"); !isPortDown(err) {
		t.Errorf("ByHandle with UserByHandle down = %v", err)
	}
}

func TestUserLookup_ReadsAUserWhoseWalletIsMissing(t *testing.T) {
	t.Parallel()
	id := ids.UserIDFrom(newStub().g.NewV7())
	noWallet := newStub(func(s *stubPorts) { s.wallet = errs.New(errs.CodeUserNotFound, "test") })
	view, err := userLookup(noWallet).ByID(t.Context(), id)
	if err != nil || view.Wallet != "" || len(view.Cabals) != 1 {
		t.Fatalf("ByID = %+v, %v", view, err)
	}
	if view, err = userLookup(newStub()).ByHandle(t.Context(), "h"); err != nil || view.Wallet != "ABCD…IJKL" {
		t.Fatalf("ByHandle = %+v, %v", view, err)
	}
	broken := newStub(func(s *stubPorts) { s.wallet = portDown() })
	if _, err := userLookup(broken).ByID(t.Context(), id); !isPortDown(err) {
		t.Fatalf("ByID with a broken wallet = %v", err)
	}
}

func TestUserLookup_RefusesAnAuthStateEventItCannotDecode(t *testing.T) {
	t.Parallel()
	bad := newStub(func(s *stubPorts) { s.payload = "not json" })
	if _, err := userLookup(bad).ByID(t.Context(), ids.UserIDFrom(bad.g.NewV7())); errs.CodeOf(err) !=
		errs.CodeDecodeFailed {
		t.Fatalf("ByID = %v, want decode_failed", err)
	}
}

func TestCabalLookup_PassesEveryPortFailureOn(t *testing.T) {
	t.Parallel()
	id := ids.CabalIDFrom(newStub().g.NewV7())
	for _, method := range []string{
		"Cabal", "Rules", "TreasuryWallet", "Members", "CabalShares", "UsersByID", "CabalHoldings", "ListAll",
		"CabalTxns", "Recent",
	} {
		if _, err := cabalLookup(newStub(failing(method))).ByID(t.Context(), id); !isPortDown(err) {
			t.Errorf("ByID with %s down = %v", method, err)
		}
	}
}

func TestShortAddress_KeepsFourCharactersOnEachEnd(t *testing.T) {
	t.Parallel()
	if got := app.ShortAddress(strings.Repeat("a", 10) + "WXYZ"); got != "aaaa…WXYZ" {
		t.Fatalf("ShortAddress = %q", got)
	}
	if got := app.ShortAddress("abcdefgh"); got != "abcdefgh" {
		t.Fatalf("ShortAddress of a short value = %q", got)
	}
}

func TestActionLog_ReportsDBUnavailableWhenTheDatabaseIsDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	log := adapters.ActionLog{DB: testkit.DB(t)}
	if _, err := log.Recent(ctx, "user", "id", 1); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Recent = %v", err)
	}
}
