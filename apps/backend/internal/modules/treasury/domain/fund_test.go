package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestFundStatus_movesOnlyForward(t *testing.T) {
	t.Parallel()
	all := []domain.FundStatus{
		domain.FundCreated, domain.FundSubmitted, domain.FundLanded, domain.FundSettled, domain.FundFailed,
		domain.FundRefunded,
	}
	allowed := map[[2]domain.FundStatus]bool{
		{domain.FundCreated, domain.FundSubmitted}: true,
		{domain.FundCreated, domain.FundFailed}:    true,
		{domain.FundSubmitted, domain.FundLanded}:  true,
		{domain.FundSubmitted, domain.FundFailed}:  true,
		{domain.FundLanded, domain.FundSettled}:    true,
		{domain.FundLanded, domain.FundRefunded}:   true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := from.CanMoveTo(to); got != allowed[[2]domain.FundStatus{from, to}] {
				t.Errorf("%s.CanMoveTo(%s) = %t", from, to, got)
			}
		}
	}
}

func TestParseFundAmount(t *testing.T) {
	t.Parallel()
	got, err := domain.ParseFundAmount("1000000")
	if err != nil || got.String() != "1000000" {
		t.Fatalf("ParseFundAmount(1000000) = %v, %v, want 1000000", got, err)
	}
	for _, raw := range []string{"999999", "0", "-5", "abc", ""} {
		_, err := domain.ParseFundAmount(raw)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseFundAmount(%q) code = %q, want invalid_input", raw, errs.CodeOf(err))
		}
	}
}

func TestNewFundRefundTxn_isABalancedFailedFundThatNetsOutOnTheWallet(t *testing.T) {
	t.Parallel()
	usdc := domain.MintAsset("usdc")
	txn, err := domain.NewFundRefundTxn(domain.UserTxnHeader{Kind: domain.UserDeposit, Status: domain.TxnSettled},
		usdc, money.MicrosFromUint64(5_000_000))
	if err != nil || txn.Kind != domain.UserFund || txn.Status != domain.TxnFailed || !txn.CabalID.IsZero() {
		t.Fatalf("txn = %+v, %v, want a failed fund with no cabal", txn.UserTxnHeader, err)
	}
	net := map[domain.UserAccount]int64{}
	for _, e := range txn.Entries() {
		net[e.Account] += e.Amount.Int64()
	}
	if len(txn.Entries()) != 4 || net[domain.UserWallet] != 0 || net[domain.UserExternal] != 0 || len(net) != 2 {
		t.Fatalf("net = %v over %d entries, want wallet and external at 0 over 4 entries", net, len(txn.Entries()))
	}
}

func TestNewFundRefundTxn_refusesAnAmountTheLedgerCannotHold(t *testing.T) {
	t.Parallel()
	usdc := domain.MintAsset("usdc")
	_, err := domain.NewFundRefundTxn(domain.UserTxnHeader{}, usdc, money.Micros{})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("zero amount code = %q, want invalid_input", errs.CodeOf(err))
	}
	_, err = domain.NewFundRefundTxn(domain.UserTxnHeader{}, usdc, money.MicrosFromUint64(math.MaxUint64))
	if err == nil {
		t.Fatal("NewFundRefundTxn accepted an amount past int64")
	}
}
