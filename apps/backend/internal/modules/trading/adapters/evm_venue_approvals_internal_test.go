package adapters

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const (
	guardUSDC   = "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	guardToken  = "0x00000000000000000000000000000000000000bb"
	guardRouter = "0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40"
	otherRouter = "0x6ff5693b99212da76ad316178a184ab56d299b43"
	strayRouter = "0x3fc91a3afd70395cd496c647d5a6cc9d4b2b7fad"
	guardAmount = 9_992_500
)

func guardAddress(t *testing.T, raw string) chain.EVMAddress {
	t.Helper()
	a, err := chain.ParseEVMAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func guardWord(v any) string {
	switch v := v.(type) {
	case string:
		return strings.Repeat("0", 24) + strings.TrimPrefix(v, "0x")
	case int64:
		return hex.EncodeToString(big.NewInt(v).FillBytes(make([]byte, abiWord)))
	}
	return hex.EncodeToString(v.(*big.Int).FillBytes(make([]byte, abiWord)))
}

func guardTx(t *testing.T, to, data string, value int64) mc.EVMTransaction {
	t.Helper()
	raw, err := hex.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	return mc.EVMTransaction{Network: chain.NetworkBase, To: guardAddress(t, to), Data: raw, Value: big.NewInt(value)}
}

func TestCheckApproval_acceptsOnlyAnExactApprovalOrPermitForThisSwap(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	window := now.Add(permitExpiryWindow).Unix()
	spec := app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: guardAddress(t, "0xf15262a7ee9d20fa690185d58e0eb2aad475bf32"),
		TokenIn: guardAddress(t, guardUSDC), TokenOut: guardAddress(t, guardToken), AmountIn: guardAmount,
	}
	erc20 := func(to, spender string, amount, value int64) mc.EVMTransaction {
		return guardTx(t, to, erc20ApproveSelector+guardWord(spender)+guardWord(amount), value)
	}
	permit := func(to, token, spender string, amount, expiry, value int64) mc.EVMTransaction {
		return guardTx(t, to,
			permit2ApproveSelector+guardWord(token)+guardWord(spender)+guardWord(amount)+guardWord(expiry), value)
	}
	dirty := "ff" + guardWord(permit2Address)[2:]
	for name, c := range map[string]struct {
		tx   mc.EVMTransaction
		want bool
	}{
		"exact approval":              {erc20(guardUSDC, permit2Address, guardAmount, 0), true},
		"approval below the amount":   {erc20(guardUSDC, permit2Address, 1, 0), true},
		"exact permit":                {permit(permit2Address, guardUSDC, guardRouter, guardAmount, window, 0), true},
		"permit expiring next":        {permit(permit2Address, guardUSDC, guardRouter, guardAmount, now.Unix()+1, 0), true},
		"approval above the amount":   {erc20(guardUSDC, permit2Address, guardAmount+1, 0), false},
		"approval to a router":        {erc20(guardUSDC, guardRouter, guardAmount, 0), false},
		"approval on another token":   {erc20(guardToken, permit2Address, guardAmount, 0), false},
		"approval with value":         {erc20(guardUSDC, permit2Address, guardAmount, 1), false},
		"approval with dirty padding": {guardTx(t, guardUSDC, erc20ApproveSelector+dirty+guardWord(int64(1)), 0), false},
		"truncated approval":          {guardTx(t, guardUSDC, erc20ApproveSelector+guardWord(permit2Address), 0), false},
		"permit above the amount":     {permit(permit2Address, guardUSDC, guardRouter, guardAmount+1, window, 0), false},
		"permit to another allowlisted router": {
			permit(permit2Address, guardUSDC, otherRouter, guardAmount, window, 0), false,
		},
		"permit to a router off the allowlist": {
			permit(permit2Address, guardUSDC, strayRouter, guardAmount, window, 0), false,
		},
		"permit of another token":    {permit(permit2Address, guardToken, guardRouter, guardAmount, window, 0), false},
		"permit not sent to Permit2": {permit(guardUSDC, guardUSDC, guardRouter, guardAmount, window, 0), false},
		"permit with value":          {permit(permit2Address, guardUSDC, guardRouter, guardAmount, window, 1), false},
		"permit already expired":     {permit(permit2Address, guardUSDC, guardRouter, guardAmount, now.Unix(), 0), false},
		"permit without expiry":      {permit(permit2Address, guardUSDC, guardRouter, guardAmount, 0, 0), false},
		"permit past the window":     {permit(permit2Address, guardUSDC, guardRouter, guardAmount, window+1, 0), false},
		"permit with dirty padding": {guardTx(t, permit2Address, permit2ApproveSelector+guardWord(guardUSDC)+
			"ff"+guardWord(guardRouter)[2:]+guardWord(int64(1))+guardWord(window), 0), false},
		"unknown selector": {guardTx(t, guardUSDC, "39509351"+guardWord(permit2Address)+guardWord(int64(1)), 0), false},
		"short calldata":   {guardTx(t, guardUSDC, "095ea7", 0), false},
		"empty calldata":   {guardTx(t, permit2Address, "", 0), false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := checkApproval(spec, guardAddress(t, guardRouter), now, c.tx)
			if c.want && err != nil || !c.want && errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("checkApproval = %v, want accepted %t", err, c.want)
			}
		})
	}
}

func TestCheckSwapTarget_refusesEveryRouterOnANetworkWithoutUniswap(t *testing.T) {
	t.Parallel()
	spec := app.EVMSwapSpec{Network: chain.NetworkSolana}
	for _, router := range []string{guardRouter, otherRouter, "0x204faca1764b154221e35c0d20abb3c525710498"} {
		tx := guardTx(t, router, "01", 0)
		if err := checkSwapTarget(spec, tx); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("checkSwapTarget(solana, %s) = %v, want decode_failed", router, err)
		}
	}
}
