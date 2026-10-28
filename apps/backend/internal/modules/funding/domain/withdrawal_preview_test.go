package domain_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestWithdrawalFeeAndArrival(t *testing.T) {
	t.Parallel()
	for network, want := range map[chain.Network]struct {
		mode    domain.FeeMode
		arrival int
	}{
		chain.NetworkSolana:   {domain.FeeOnTop, 60},
		chain.NetworkBase:     {domain.FeeIncluded, 120},
		chain.NetworkArbitrum: {domain.FeeIncluded, 120},
		chain.NetworkEthereum: {domain.FeeIncluded, 300},
	} {
		fee, mode := domain.WithdrawalFee(network)
		if !fee.IsZero() || mode != want.mode || domain.ArrivalSeconds(network) != want.arrival {
			t.Errorf("%s: fee %s mode %s arrival %d", network, fee, mode, domain.ArrivalSeconds(network))
		}
	}
}

func TestMaxSendableExamples(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		available, fee, want uint64
		mode                 domain.FeeMode
	}{
		"cash less the fee":          {500_000_000, 500_000, 499_500_000, domain.FeeOnTop},
		"no fee":                     {500_000_000, 0, 500_000_000, domain.FeeOnTop},
		"zero balance":               {0, 500_000, 0, domain.FeeOnTop},
		"fee larger than the cash":   {100_000, 500_000, 0, domain.FeeOnTop},
		"fee equal to the cash":      {500_000, 500_000, 0, domain.FeeOnTop},
		"included takes all of it":   {500_000_000, 7_500, 500_000_000, domain.FeeIncluded},
		"included with no cash":      {0, 7_500, 0, domain.FeeIncluded},
		"included fee beyond cash":   {1_000, 7_500, 1_000, domain.FeeIncluded},
		"on top with the whole cash": {1, 0, 1, domain.FeeOnTop},
	} {
		got := domain.MaxSendable(money.MicrosFromUint64(c.available), money.MicrosFromUint64(c.fee), c.mode)
		if got.Uint64() != c.want {
			t.Errorf("%s: max = %s, want %d", name, got, c.want)
		}
	}
}

func TestMaxSendableProperty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		available := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<50).Draw(t, "available"))
		fee := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<50).Draw(t, "fee"))
		got := domain.MaxSendable(available, fee, domain.FeeOnTop)
		if got.Cmp(available) > 0 {
			t.Fatalf("max %s above available %s", got, available)
		}
		if available.Cmp(fee) < 0 {
			if !got.IsZero() {
				t.Fatalf("max %s with fee %s above available %s", got, fee, available)
			}
			return
		}
		left, _ := available.Sub(fee)
		total, err := got.Add(fee)
		if got != left || err != nil || total.Cmp(available) > 0 {
			t.Fatalf("max %s, want %s, plus fee %s is %s (%v)", got, left, fee, total, err)
		}
	})
}

func TestMaxSendableIncludedIsTheWholeBalance(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		available := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<50).Draw(t, "available"))
		fee := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<50).Draw(t, "fee"))
		if got := domain.MaxSendable(available, fee, domain.FeeIncluded); got != available {
			t.Fatalf("included max %s, want available %s", got, available)
		}
	})
}

func TestParseWithdrawalPreview(t *testing.T) {
	t.Parallel()
	amount, small := "650000000", "500000"
	for name, c := range map[string]struct {
		raw         domain.RawWithdrawalPreview
		network     chain.Network
		amount, evm string
	}{
		"empty":               {domain.RawWithdrawalPreview{}, chain.NetworkSolana, "", ""},
		"solana with address": {domain.RawWithdrawalPreview{Amount: &amount, To: wallet}, chain.NetworkSolana, amount, ""},
		"evm without address": {
			domain.RawWithdrawalPreview{Amount: &small, Kind: "evm", Network: "base"}, chain.NetworkBase, small, "",
		},
		"evm with address": {
			domain.RawWithdrawalPreview{Kind: "evm", Network: "ethereum", To: evmMixed}, chain.NetworkEthereum, "", evmLower,
		},
	} {
		got, err := domain.ParseWithdrawalPreview(c.raw)
		gotAmount, gotEVM := "", ""
		if got.Amount != nil {
			gotAmount = got.Amount.String()
		}
		if got.EVM != nil {
			gotEVM = got.EVM.String()
		}
		if err != nil || got.Network != c.network || gotAmount != c.amount || gotEVM != c.evm {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestParseWithdrawalPreview_Refusals(t *testing.T) {
	t.Parallel()
	const zero = "0x0000000000000000000000000000000000000000"
	bad := "1.5"
	for name, c := range map[string]struct {
		raw  domain.RawWithdrawalPreview
		want errs.Code
	}{
		"not a number":          {domain.RawWithdrawalPreview{Amount: &bad}, errs.CodeInvalidInput},
		"bad solana address":    {domain.RawWithdrawalPreview{To: "0OIl"}, errs.CodeInvalidAddress},
		"evm address on solana": {domain.RawWithdrawalPreview{To: evmLower}, errs.CodeInvalidAddress},
		"zero evm address":      {domain.RawWithdrawalPreview{Kind: "evm", Network: "base", To: zero}, errs.CodeInvalidAddress},
		"solana kind on base":   {domain.RawWithdrawalPreview{Network: "base"}, errs.CodeInvalidNetwork},
		"evm on robinhood":      {domain.RawWithdrawalPreview{Kind: "evm", Network: "robinhood"}, errs.CodeInvalidNetwork},
		"unknown network":       {domain.RawWithdrawalPreview{Network: "polygon"}, errs.CodeInvalidNetwork},
		"unknown kind":          {domain.RawWithdrawalPreview{Kind: "bank"}, errs.CodeInvalidInput},
	} {
		if _, err := domain.ParseWithdrawalPreview(c.raw); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}
