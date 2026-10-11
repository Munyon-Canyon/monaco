package adapters

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const (
	permit2Address         = "0x000000000022d473030f116ddee9f6b43ac78ba3"
	erc20ApproveSelector   = "095ea7b3"
	permit2ApproveSelector = "87517c45"
	abiWord                = 32
	addressPadding         = 12
)

func universalRouters(n chain.Network) []string {
	switch n {
	case chain.NetworkBase:
		return []string{
			"0x6ff5693b99212da76ad316178a184ab56d299b43", "0xfdf682f51fe81aa4898f0ae2163d8a55c127fbc7",
			"0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40",
		}
	case chain.NetworkEthereum:
		return []string{
			"0x66a9893cc07d91d95644aedd05d03f95e1dba8af", "0x4c82d1fbfe28c977cbb58d8c7ff8fcf9f70a2cca",
			"0x23617e59a5925b2a4bf75d73ff6711cd0b29de85",
		}
	case chain.NetworkArbitrum:
		return []string{
			"0xa51afafe0263b40edaef0df8781ea9aa03e381a3", "0x8b844f885672f333bc0042cb669255f93a4c1e6b",
			"0x2d01411773c8c24805306e89a41f7855c3c4fe65",
		}
	case chain.NetworkSolana, chain.NetworkRobinhood:
	}
	return nil
}

func checkSwapTarget(spec app.EVMSwapSpec, tx mc.EVMTransaction) error {
	if !slices.Contains(universalRouters(spec.Network), tx.To.String()) {
		return errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.swapTarget", slog.String("to", tx.To.String()))
	}
	return nil
}

func checkApproval(spec app.EVMSwapSpec, router chain.EVMAddress, tx mc.EVMTransaction) error {
	refused := errs.New(errs.CodeDecodeFailed, "trading.EVMVenue.approval", slog.String("to", tx.To.String()))
	permit2, _ := chain.ParseEVMAddress(permit2Address)
	if len(tx.Data) < 4 {
		return refused
	}
	selector, args := hex.EncodeToString(tx.Data[:4]), tx.Data[4:]
	switch selector {
	case erc20ApproveSelector:
		spender, ok := wordAddress(args, 0, len(args) == 2*abiWord)
		amount := new(big.Int).SetBytes(args[min(len(args), abiWord):])
		return approved(ok && tx.To == spec.TokenIn && spender == permit2, amount, spec, refused)
	case permit2ApproveSelector:
		token, ok := wordAddress(args, 0, len(args) == 4*abiWord && tx.To == permit2)
		spender, ok := wordAddress(args, 1, ok)
		amount := new(big.Int).SetBytes(args[min(len(args), 2*abiWord):min(len(args), 3*abiWord)])
		return approved(ok && token == spec.TokenIn && spender == router, amount, spec, refused)
	}
	return refused
}

func wordAddress(args []byte, index int, ok bool) (chain.EVMAddress, bool) {
	var a chain.EVMAddress
	if !ok {
		return a, false
	}
	word := args[index*abiWord : (index+1)*abiWord]
	copy(a[:], word[addressPadding:])
	return a, bytes.Equal(word[:addressPadding], make([]byte, addressPadding))
}

func approved(ok bool, amount *big.Int, spec app.EVMSwapSpec, refused error) error {
	if !ok || amount.Cmp(new(big.Int).SetUint64(spec.AmountIn)) > 0 {
		return refused
	}
	return nil
}
