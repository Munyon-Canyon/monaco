package admin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
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
	found   bool
	swapErr error
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

func (s stubPorts) LatestCabalValues(context.Context) ([]rankingport.CabalValue, error) {
	return nil, s.err("LatestCabalValues")
}

func (s stubPorts) IsPaused(context.Context, ids.CabalID) (fundingport.Pause, error) {
	return fundingport.Pause{}, s.err("IsPaused")
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

func (s stubPorts) TxnByID(context.Context, uuid.UUID) (treasuryport.Txn, bool, error) {
	return s.txn(), s.found, s.err("TxnByID")
}

func (s stubPorts) TxnBySignature(context.Context, chain.Signature) (treasuryport.Txn, bool, error) {
	return s.txn(), s.found, s.err("TxnBySignature")
}

func (s stubPorts) txn() treasuryport.Txn {
	swap := ids.SwapIDFrom(s.g.NewV7())
	return treasuryport.Txn{TxnHeader: treasuryport.TxnHeader{SwapID: &swap}}
}

func (s stubPorts) Swap(context.Context, ids.SwapID) (trading.SwapView, error) {
	if s.swapErr != nil {
		return trading.SwapView{}, s.swapErr
	}
	return trading.SwapView{ID: ids.SwapIDFrom(s.g.NewV7())}, s.err("Swap")
}

func (s stubPorts) SwapBySignature(context.Context, chain.Signature) (trading.SwapView, error) {
	if s.swapErr != nil {
		return trading.SwapView{}, s.swapErr
	}
	return trading.SwapView{ID: ids.SwapIDFrom(s.g.NewV7())}, s.err("SwapBySignature")
}

func (s stubPorts) ExecuteRequestID(context.Context, ids.SwapID) (string, error) {
	return "req", s.err("ExecuteRequestID")
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
		Cabals: s, Details: s, Users: s, Shares: s, Holdings: s, Assets: s, Values: s, Pauses: s, Txns: s,
		Actions: s,
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
		"LatestCabalValues", "IsPaused", "CabalTxns", "Recent",
	} {
		if _, err := cabalLookup(newStub(failing(method))).ByID(t.Context(), id); !isPortDown(err) {
			t.Errorf("ByID with %s down = %v", method, err)
		}
	}
}

func txnLookup(s stubPorts) app.TxnLookup {
	return app.TxnLookup{Ledger: s, Swaps: s, Requests: s, Events: s}
}

func TestTxnLookup_PassesEveryPortFailureOn(t *testing.T) {
	t.Parallel()
	found := func(s *stubPorts) { s.found = true }
	for _, method := range []string{"TxnByID", "Swap", "ExecuteRequestID", "EventsByAggregate"} {
		if _, err := txnLookup(newStub(found, failing(method))).ByID(t.Context(), uuid.UUID{}); !isPortDown(err) {
			t.Errorf("ByID with %s down = %v", method, err)
		}
	}
	for _, method := range []string{"TxnBySignature", "Swap", "ExecuteRequestID", "EventsByAggregate"} {
		if _, err := txnLookup(newStub(found, failing(method))).BySignature(t.Context(), "sig"); !isPortDown(err) {
			t.Errorf("BySignature with %s down = %v", method, err)
		}
	}
	for _, method := range []string{"Swap", "ExecuteRequestID", "EventsByAggregate"} {
		if _, err := txnLookup(newStub(failing(method))).ByID(t.Context(), uuid.UUID{}); !isPortDown(err) {
			t.Errorf("swap-only ByID with %s down = %v", method, err)
		}
	}
	if _, err := txnLookup(newStub(failing("SwapBySignature"))).BySignature(t.Context(), "sig"); !isPortDown(err) {
		t.Errorf("BySignature with SwapBySignature down = %v", err)
	}
}

func TestTxnLookup_AMissingSwapIsAbsentFromALedgerLookupButNotFoundOnItsOwn(t *testing.T) {
	t.Parallel()
	gone := errs.New(errs.CodeSwapNotFound, "test")
	withLedger := newStub(func(s *stubPorts) { s.found, s.swapErr = true, gone })
	view, err := txnLookup(withLedger).ByID(t.Context(), uuid.UUID{})
	if err != nil || view.Ledger == nil || view.Swap != nil {
		t.Fatalf("ByID = %+v, %v", view, err)
	}
	alone := newStub(func(s *stubPorts) { s.swapErr = gone })
	if _, err := txnLookup(alone).ByID(t.Context(), uuid.UUID{}); errs.CodeOf(err) != errs.CodeTxnNotFound {
		t.Fatalf("ByID = %v, want txn_not_found", err)
	}
	if _, err := txnLookup(alone).BySignature(t.Context(), "sig"); errs.CodeOf(err) != errs.CodeTxnNotFound {
		t.Fatalf("BySignature = %v, want txn_not_found", err)
	}
}

func TestTxnView_SignaturePrefersTheLedgerThenTheSwap(t *testing.T) {
	t.Parallel()
	swap := &app.SwapDetail{TxSignature: "swap-sig"}
	if got := (app.TxnView{Swap: swap}).Signature(); got != "swap-sig" {
		t.Fatalf("Signature = %q", got)
	}
	ledger := &app.LedgerTxn{TxnHeader: app.TxnHeader{TxSignature: "ledger-sig"}}
	if got := (app.TxnView{Ledger: ledger, Swap: swap}).Signature(); got != "ledger-sig" {
		t.Fatalf("Signature = %q", got)
	}
	if got := (app.TxnView{Ledger: &app.LedgerTxn{}}).Signature(); got != "" {
		t.Fatalf("Signature = %q", got)
	}
}

func (s stubPorts) Stuck(context.Context, time.Duration, int) ([]trading.SwapView, error) {
	return []trading.SwapView{{}}, s.err("Stuck")
}

func (s stubPorts) CountStuck(context.Context, time.Duration) (int, error) {
	return 1, s.err("CountStuck")
}

func (s stubPorts) Unpublished(context.Context, time.Duration, int) ([]bus.StaleEvent, error) {
	return []bus.StaleEvent{{}}, s.err("Unpublished")
}

func (s stubPorts) CountUnpublished(context.Context, time.Duration) (int, error) {
	return 1, s.err("CountUnpublished")
}

func (s stubPorts) CountOpen(context.Context) (int, error) { return 1, s.err("CountOpen") }

func queues(s stubPorts) app.Queues {
	return app.Queues{Swaps: s, Events: s, Letters: s, Clock: testkit.NewClock(time.Unix(0, 0))}
}

func TestQueues_PassEveryPortFailureOn(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"CountStuck", "CountUnpublished", "CountOpen"} {
		if _, err := queues(newStub(failing(method))).Counts(t.Context()); !isPortDown(err) {
			t.Errorf("Counts with %s down = %v", method, err)
		}
	}
	if _, err := queues(newStub(failing("Stuck"))).StuckTxns(t.Context(), time.Minute, 1); !isPortDown(err) {
		t.Errorf("StuckTxns = %v", err)
	}
	if _, err := queues(newStub(failing("Unpublished"))).UnpublishedEvents(t.Context(), time.Minute, 1); !isPortDown(
		err,
	) {
		t.Errorf("UnpublishedEvents = %v", err)
	}
}

func TestParseOlderThan_DefaultsWhenEmptyAndRefusesAnythingOutOfRange(t *testing.T) {
	t.Parallel()
	if got, err := app.ParseOlderThan("", time.Minute); err != nil || got != time.Minute {
		t.Fatalf("ParseOlderThan(empty) = %v, %v", got, err)
	}
	if got, err := app.ParseOlderThan("90s", time.Minute); err != nil || got != 90*time.Second {
		t.Fatalf("ParseOlderThan(90s) = %v, %v", got, err)
	}
	for _, raw := range []string{"soon", "0s", "-1m", "24h1s"} {
		if _, err := app.ParseOlderThan(raw, time.Minute); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseOlderThan(%q) = %v", raw, err)
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
