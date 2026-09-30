package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const usdc = domain.Asset("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

func amount(v int64) money.SignedMicros { return money.SignedMicrosFromInt64(v) }

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want code %q", code)
	}
	if got := errs.CodeOf(err); got != code {
		t.Fatalf("code = %q, want %q (err %v)", got, code, err)
	}
}

func TestNewCabalTxn_keepsABalancedSwap(t *testing.T) {
	t.Parallel()
	aapl := domain.MintAsset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	entries := []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-50_000_000)},
		{Account: domain.CabalTreasury, Asset: aapl, Amount: amount(210_000)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: amount(50_000_000)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: amount(-210_000)},
	}
	h := domain.CabalTxnHeader{ID: testkit.NewIDs(1).NewV7(), Kind: domain.CabalSwap, Status: domain.TxnSettled}
	txn, err := domain.NewCabalTxn(h, entries)
	if err != nil {
		t.Fatal(err)
	}
	got := txn.Entries()
	got[0].Amount = amount(1)
	if txn.Entries()[0].Amount != amount(-50_000_000) || txn.CabalTxnHeader != h || len(got) != 4 {
		t.Fatalf("txn = %+v, want the header and an unaliased copy of the entries", txn)
	}
}

func TestNewTxn_rejectsEmptyZeroAndUnbalancedHeaders(t *testing.T) {
	t.Parallel()
	shares := domain.SharesAsset(cabalID(t, 2))
	tests := map[string]struct {
		entries []domain.UserEntry
		code    errs.Code
	}{
		"no entries":  {nil, errs.CodeInvalidInput},
		"zero amount": {[]domain.UserEntry{{Account: domain.UserWallet, Asset: usdc}}, errs.CodeInvalidInput},
		"no asset":    {[]domain.UserEntry{{Account: domain.UserWallet, Amount: amount(1)}}, errs.CodeInvalidInput},
		"one side": {[]domain.UserEntry{
			{Account: domain.UserWallet, Asset: usdc, Amount: amount(5)},
			{Account: domain.UserExternal, Asset: usdc, Amount: amount(-4)},
		}, errs.CodeLedgerUnbalanced},
		"assets cross": {[]domain.UserEntry{
			{Account: domain.UserHolder, Asset: shares, Amount: amount(5)},
			{Account: domain.UserWallet, Asset: usdc, Amount: amount(-5)},
		}, errs.CodeLedgerUnbalanced},
		"sum wraps int64 to zero": {[]domain.UserEntry{
			{Account: domain.UserWallet, Asset: usdc, Amount: amount(math.MaxInt64)},
			{Account: domain.UserExternal, Asset: usdc, Amount: amount(math.MaxInt64)},
			{Account: domain.UserCabal, Asset: usdc, Amount: amount(2)},
		}, errs.CodeLedgerUnbalanced},
		"too many": {make([]domain.UserEntry, 1<<15), errs.CodeInvalidInput},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewUserTxn(domain.UserTxnHeader{Kind: domain.UserDeposit}, tt.entries)
			wantCode(t, err, tt.code)
			_, err = domain.NewCabalTxn(domain.CabalTxnHeader{Kind: domain.CabalFund}, cabalSide(tt.entries))
			wantCode(t, err, tt.code)
		})
	}
}

func cabalID(t *testing.T, seed uint64) ids.CabalID {
	t.Helper()
	id, err := ids.ParseCabalID(testkit.NewIDs(seed).NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func cabalSide(user []domain.UserEntry) []domain.CabalEntry {
	out := make([]domain.CabalEntry, len(user))
	for i, e := range user {
		out[i] = domain.CabalEntry{Account: domain.CabalMembers, Asset: e.Asset, Amount: e.Amount}
	}
	return out
}

func TestTxnStatus_movesOnlyFromPending(t *testing.T) {
	t.Parallel()
	all := []domain.TxnStatus{domain.TxnPending, domain.TxnSettled, domain.TxnFailed}
	for _, from := range all {
		for _, to := range all {
			want := from == domain.TxnPending && to != domain.TxnPending
			if got := from.CanMoveTo(to); got != want {
				t.Errorf("%s -> %s = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestSharesAsset_namesTheCabal(t *testing.T) {
	t.Parallel()
	cabal := cabalID(t, 3)
	if got := domain.SharesAsset(cabal); got != domain.Asset("shares:"+cabal.String()) {
		t.Fatalf("SharesAsset = %q", got)
	}
}

func TestNewUserTxn_keepsABalancedDeposit(t *testing.T) {
	t.Parallel()
	entries := []domain.UserEntry{
		{Account: domain.UserExternal, Asset: usdc, Amount: amount(-25_000_000)},
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(25_000_000)},
	}
	h := domain.UserTxnHeader{ID: testkit.NewIDs(4).NewV7(), Kind: domain.UserDeposit, Status: domain.TxnSettled}
	txn, err := domain.NewUserTxn(h, entries)
	if err != nil {
		t.Fatal(err)
	}
	got := txn.Entries()
	got[1].Account = domain.UserHolder
	if txn.UserTxnHeader != h || txn.Entries()[1] != entries[1] {
		t.Fatalf("txn = %+v, want the header and an unaliased copy of the entries", txn)
	}
}
