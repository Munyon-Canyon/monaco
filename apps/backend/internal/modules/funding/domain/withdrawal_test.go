package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	wallet      = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	usdcAccount = "FGETo8T8wMcN2wCjav8VK6eh3dLk63evNDPxzLSJra8B"
	evmMixed    = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	evmLower    = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
)

func TestParseWithdrawalRequest(t *testing.T) {
	t.Parallel()
	got, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{Amount: "1000000", To: wallet})
	if err != nil || got.Amount.String() != "1000000" || string(got.To) != wallet ||
		got.Network != chain.NetworkSolana || got.Kind() != domain.DestinationSolana || got.Address() != wallet {
		t.Fatalf("Parse = %+v, %v", got, err)
	}
	for name, c := range map[string]struct {
		amount, to string
		want       errs.Code
	}{
		"below minimum": {"999999", wallet, errs.CodeInvalidInput},
		"not a number":  {"1.5", wallet, errs.CodeInvalidInput},
		"negative":      {"-1000000", wallet, errs.CodeInvalidInput},
		"bad address":   {"1000000", "0OIl", errs.CodeInvalidAddress},
		"token account": {"1000000", usdcAccount, errs.CodeInvalidAddress},
		"evm address":   {"1000000", evmLower, errs.CodeInvalidAddress},
	} {
		raw := domain.RawWithdrawal{Amount: c.amount, To: c.to}
		if _, err := domain.ParseWithdrawalRequest(raw); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}

func TestParseWithdrawalRequest_EVM(t *testing.T) {
	t.Parallel()
	for _, network := range []chain.Network{chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum} {
		got, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{
			Amount: "1000000", Kind: "evm", Network: string(network), To: evmMixed,
		})
		if err != nil || got.Network != network || got.Kind() != domain.DestinationEVM ||
			got.Address() != evmLower || got.To != "" {
			t.Fatalf("Parse(%s) = %+v, %v", network, got, err)
		}
	}
}

func TestParseWithdrawalRequest_RefusesBadDestinations(t *testing.T) {
	t.Parallel()
	const zero = "0x0000000000000000000000000000000000000000"
	for name, c := range map[string]struct {
		kind, network, to string
		want              errs.Code
	}{
		"evm without network":    {"evm", "", evmLower, errs.CodeInvalidNetwork},
		"evm on solana":          {"evm", "solana", evmLower, errs.CodeInvalidNetwork},
		"evm on robinhood":       {"evm", "robinhood", evmLower, errs.CodeInvalidNetwork},
		"unknown network":        {"evm", "polygon", evmLower, errs.CodeInvalidNetwork},
		"solana kind on base":    {"", "base", wallet, errs.CodeInvalidNetwork},
		"bad checksum":           {"evm", "base", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD", errs.CodeInvalidAddress},
		"short evm address":      {"evm", "base", evmLower[:40], errs.CodeInvalidAddress},
		"solana address on base": {"evm", "base", wallet, errs.CodeInvalidAddress},
		"zero address":           {"evm", "base", zero, errs.CodeInvalidAddress},
		"unknown kind":           {"bank", "", wallet, errs.CodeInvalidInput},
	} {
		raw := domain.RawWithdrawal{Amount: "1000000", Kind: c.kind, Network: c.network, To: c.to}
		if _, err := domain.ParseWithdrawalRequest(raw); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}
