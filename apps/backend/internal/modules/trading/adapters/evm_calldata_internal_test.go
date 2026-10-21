package adapters

import (
	"math/big"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/evmcalls"
)

const (
	cdOwner    = "0xf15262a7ee9d20fa690185d58e0eb2aad475bf32"
	cdAttacker = "0x00000000000000000000000000000000000000ee"
	cdOther    = "0x00000000000000000000000000000000000000dd"
	cdHolds    = "0x0000000000000000000000000000000000000002"
	cdSender   = "0x0000000000000000000000000000000000000001"
	cdFloor    = 990_000
)

func cdSpec(t *testing.T) app.EVMSwapSpec {
	t.Helper()
	return app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: guardAddress(t, cdOwner), TokenIn: guardAddress(t, guardUSDC),
		TokenOut: guardAddress(t, guardToken), AmountIn: guardAmount, MinOut: cdFloor,
	}
}

func cdTx(t *testing.T, data []byte) mc.EVMTransaction {
	t.Helper()
	return mc.EVMTransaction{
		Network: chain.NetworkBase, To: guardAddress(t, guardRouter), Data: data, Value: new(big.Int),
	}
}

func cdV3(recipient string, amountIn, minOut uint64, payer bool, tokens ...string) []byte {
	return evmcalls.V3In(recipient, amountIn, minOut, evmcalls.V3Path(tokens...), payer)
}

func cdRefused(t *testing.T, name string, data []byte) {
	t.Helper()
	err := checkSwapCalldata(cdSpec(t), cdTx(t, data), cdFloor)
	if err == nil {
		t.Fatalf("%s: calldata was accepted", name)
	}
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("%s: err = %v, want decode_failed", name, err)
	}
}

func TestCheckSwapCalldata_acceptsEachShapeAnHonestQuoteProduces(t *testing.T) {
	t.Parallel()
	spec := cdSpec(t)
	in, out := guardUSDC, guardToken
	wrapped := evmcalls.V4([]byte{0x06, 0x0c, 0x0f},
		evmcalls.V4Single(in, out, true, guardAmount, 1),
		evmcalls.V4Pair(in, guardAmount), evmcalls.V4Pair(out, 995_000))
	path := evmcalls.V4([]byte{0x07, 0x0c, 0x0f},
		evmcalls.V4Path(in, []string{cdOther, out}, guardAmount, 1),
		evmcalls.V4Pair(in, guardAmount), evmcalls.V4Pair(out, 995_000))
	reverse := evmcalls.V4([]byte{0x06, 0x0c, 0x0f},
		evmcalls.V4Single(out, in, false, guardAmount, 1),
		evmcalls.V4Pair(in, guardAmount), evmcalls.V4Pair(out, 995_000))
	for name, data := range map[string][]byte{
		"v3 to the owner": evmcalls.Honest(cdOwner, in, out, guardAmount, 995_000),
		"v3 multi hop":    evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount, 995_000, true, in, cdOther, out)),
		"v2 to the owner": evmcalls.Execute([]byte{0x08}, evmcalls.V2In(cdOwner, guardAmount, 995_000, []string{in, out}, true)),
		"no deadline":     evmcalls.ExecuteNoDeadline([]byte{0x00}, cdV3(cdOwner, guardAmount, 995_000, true, in, out)),
		"to the sender":   evmcalls.Execute([]byte{0x00}, cdV3(cdSender, guardAmount, 995_000, true, in, out)),
		"router then sweep": evmcalls.Execute([]byte{0x02, 0x00, 0x04},
			evmcalls.Permit2Transfer(in, cdHolds, guardAmount),
			cdV3(cdHolds, guardAmount, 1, false, in, out), evmcalls.Sweep(out, cdOwner, 995_000)),
		"split routes": evmcalls.Execute([]byte{0x00, 0x08},
			cdV3(cdOwner, guardAmount/2, 495_000, true, in, out),
			evmcalls.V2In(cdOwner, guardAmount/2, 495_000, []string{in, out}, true)),
		"refund of the input": evmcalls.Execute([]byte{0x00, 0x04},
			cdV3(cdOwner, guardAmount, 995_000, true, in, out), evmcalls.Sweep(in, cdOwner, 0)),
		"balance check": evmcalls.Execute([]byte{0x0e, 0x00}, evmcalls.Sweep(out, cdOwner, 0),
			cdV3(cdOwner, guardAmount, 995_000, true, in, out)),
		"v4 single":            evmcalls.Execute([]byte{0x10}, wrapped),
		"v4 path":              evmcalls.Execute([]byte{0x10}, path),
		"v4 reverse direction": evmcalls.Execute([]byte{0x10}, reverse),
	} {
		if err := checkSwapCalldata(spec, cdTx(t, data), cdFloor); err != nil {
			t.Errorf("%s: honest calldata refused: %v %v", name, err, errs.Detail(err))
		}
	}
}

func TestCheckSwapCalldata_refusesMaliciousCalldataBehindValidLookingMetadata(t *testing.T) {
	t.Parallel()
	in, out := guardUSDC, guardToken
	honest := cdV3(cdOwner, guardAmount, 995_000, true, in, out)
	v4 := func(actions []byte, params ...[]byte) []byte {
		return evmcalls.Execute([]byte{0x10}, evmcalls.V4(actions, params...))
	}
	single := evmcalls.V4Single(in, out, true, guardAmount, 1)
	settle, take := evmcalls.V4Pair(in, guardAmount), evmcalls.V4Pair(out, 995_000)
	for name, data := range map[string][]byte{
		"wrong output token":      evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount, 995_000, true, in, cdOther)),
		"wrong input token":       evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount, 995_000, true, cdOther, out)),
		"amount above approval":   evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount+1, 995_000, true, in, out)),
		"wrong recipient":         evmcalls.Execute([]byte{0x00}, cdV3(cdAttacker, guardAmount, 995_000, true, in, out)),
		"v2 wrong recipient":      evmcalls.Execute([]byte{0x08}, evmcalls.V2In(cdAttacker, guardAmount, 995_000, []string{in, out}, true)),
		"v2 wrong output":         evmcalls.Execute([]byte{0x08}, evmcalls.V2In(cdOwner, guardAmount, 995_000, []string{in, cdOther}, true)),
		"v2 short path":           evmcalls.Execute([]byte{0x08}, evmcalls.V2In(cdOwner, guardAmount, 995_000, []string{in}, true)),
		"sweep to a third party":  evmcalls.Execute([]byte{0x00, 0x04}, cdV3(cdHolds, guardAmount, 1, true, in, out), evmcalls.Sweep(out, cdAttacker, 995_000)),
		"sweep of another token":  evmcalls.Execute([]byte{0x00, 0x04}, cdV3(cdOwner, guardAmount, 995_000, true, in, out), evmcalls.Sweep(cdOther, cdOwner, 1)),
		"extra pull to attacker":  evmcalls.Execute([]byte{0x02, 0x00}, evmcalls.Permit2Transfer(in, cdAttacker, guardAmount), honest),
		"extra pull of the input": evmcalls.Execute([]byte{0x02, 0x00}, evmcalls.Permit2Transfer(in, cdHolds, guardAmount), honest),
		"pull of another token":   evmcalls.Execute([]byte{0x02, 0x00}, evmcalls.Permit2Transfer(cdOther, cdHolds, 1), honest),
		"plain transfer command":  evmcalls.Execute([]byte{0x05, 0x00}, evmcalls.Sweep(in, cdAttacker, 1), honest),
		"pay portion skim":        evmcalls.Execute([]byte{0x00, 0x06}, honest, evmcalls.Sweep(out, cdAttacker, 100)),
		"permit change":           evmcalls.Execute([]byte{0x0a, 0x00}, make([]byte, 5*32), honest),
		"batch permit change":     evmcalls.Execute([]byte{0x03, 0x00}, make([]byte, 5*32), honest),
		"wrap and unwrap":         evmcalls.Execute([]byte{0x0b, 0x00}, evmcalls.Sweep(out, cdOwner, 1), honest),
		"may revert silently":     evmcalls.Execute([]byte{0x80}, honest),
		"minimum below floor":     evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount, cdFloor-1, true, in, out)),
		"output left in router":   evmcalls.Execute([]byte{0x00}, cdV3(cdHolds, guardAmount, 995_000, true, in, out)),
		"router minimum ignored":  evmcalls.Execute([]byte{0x00, 0x04}, cdV3(cdHolds, guardAmount, 995_000, true, in, out), evmcalls.Sweep(out, cdOwner, 1)),
		"pulls twice":             evmcalls.Execute([]byte{0x02, 0x00}, evmcalls.Permit2Transfer(in, cdHolds, guardAmount), cdV3(cdOwner, guardAmount, 995_000, true, in, out)),
		"payer flag out of range": evmcalls.Execute([]byte{0x00}, evmcalls.Heads(
			[][]byte{evmcalls.AddrWord(cdOwner), evmcalls.Uint(guardAmount), evmcalls.Uint(995_000), nil, evmcalls.Uint(2)},
			[][]byte{nil, nil, nil, evmcalls.Bytes(evmcalls.V3Path(in, out)), nil})),
		"no swap at all":         evmcalls.Execute([]byte{0x04}, evmcalls.Sweep(out, cdOwner, 995_000)),
		"empty":                  evmcalls.Execute(nil),
		"v3 path with no hop":    evmcalls.Execute([]byte{0x00}, cdV3(cdOwner, guardAmount, 995_000, true, in)),
		"v4 wrong input":         v4([]byte{0x06, 0x0c, 0x0f}, evmcalls.V4Single(cdOther, out, true, guardAmount, 1), settle, take),
		"v4 wrong output":        v4([]byte{0x06, 0x0c, 0x0f}, evmcalls.V4Single(in, cdOther, true, guardAmount, 1), settle, take),
		"v4 amount above":        v4([]byte{0x06, 0x0c, 0x0f}, evmcalls.V4Single(in, out, true, guardAmount+1, 1), settle, take),
		"v4 settle above":        v4([]byte{0x06, 0x0c, 0x0f}, single, evmcalls.V4Pair(in, guardAmount+1), take),
		"v4 settle another":      v4([]byte{0x06, 0x0c, 0x0f}, single, evmcalls.V4Pair(cdOther, guardAmount), take),
		"v4 take another":        v4([]byte{0x06, 0x0c, 0x0f}, single, settle, evmcalls.V4Pair(cdOther, 995_000)),
		"v4 take below floor":    v4([]byte{0x06, 0x0c, 0x0f}, single, settle, evmcalls.V4Pair(out, cdFloor-1)),
		"v4 take to a recipient": v4([]byte{0x06, 0x0c, 0x0e}, single, settle, evmcalls.Sweep(out, cdAttacker, 995_000)),
		"v4 portion skim":        v4([]byte{0x06, 0x0c, 0x10, 0x0f}, single, settle, evmcalls.Sweep(out, cdAttacker, 100), take),
		"v4 path wrong output":   v4([]byte{0x07, 0x0c, 0x0f}, evmcalls.V4Path(in, []string{cdOther}, guardAmount, 1), settle, take),
		"v4 path without hops":   v4([]byte{0x07, 0x0c, 0x0f}, evmcalls.V4Path(in, nil, guardAmount, 1), settle, take),
		"v4 params mismatch":     v4([]byte{0x06, 0x0c}, single),
		"v4 params past the data": evmcalls.Execute([]byte{0x10}, evmcalls.Heads([][]byte{nil, nil},
			[][]byte{evmcalls.Bytes([]byte{0x06}), slices.Concat(evmcalls.Uint(1), evmcalls.Uint(32))})),
		"v4 path hop with junk": evmcalls.Execute([]byte{0x10}, evmcalls.V4([]byte{0x07, 0x0c, 0x0f},
			junked(evmcalls.V4Path(in, []string{out}, guardAmount, 1), 224), settle, take)),
		"recipient with junk": evmcalls.Execute([]byte{0x00}, junked(cdV3(cdOwner, guardAmount, 995_000, true, in, out), 0)),
		"v2 path with junk":   evmcalls.Execute([]byte{0x08}, junked(evmcalls.V2In(cdOwner, guardAmount, 995_000, []string{in, out}, true), 6*32)),
		"input array past the data": slices.Concat([]byte{0x35, 0x93, 0x56, 0x4c}, evmcalls.Heads([][]byte{nil, nil, evmcalls.Uint(1)},
			[][]byte{evmcalls.Bytes([]byte{0}), slices.Concat(evmcalls.Uint(1), evmcalls.Uint(1<<40)), nil})),
		"v4 truncated params": v4([]byte{0x06, 0x0c, 0x0f}, []byte{1, 2, 3}, settle, take),
	} {
		cdRefused(t, name, data)
	}
}

func junked(b []byte, at int) []byte {
	out := slices.Clone(b)
	out[at] = 1
	return out
}

func TestCheckSwapCalldata_refusesMalformedCalldata(t *testing.T) {
	t.Parallel()
	honest := evmcalls.Honest(cdOwner, guardUSDC, guardToken, guardAmount, 995_000)
	for name, data := range map[string][]byte{
		"empty":                     nil,
		"selector only":             honest[:4],
		"another selector":          append([]byte{1, 2, 3, 4}, honest[4:]...),
		"truncated":                 honest[:len(honest)-40],
		"offset past the data":      append(append([]byte(nil), honest[:4]...), evmcalls.Uint(1<<40)...),
		"more commands than inputs": evmcalls.Execute([]byte{0x00, 0x00}, cdV3(cdOwner, guardAmount, 995_000, true, guardUSDC, guardToken)),
		"huge array": append(append([]byte(nil), honest[:4]...), evmcalls.Heads(
			[][]byte{nil, nil, evmcalls.Uint(1)}, [][]byte{evmcalls.Bytes([]byte{0}), evmcalls.Uint(1 << 20), nil})...),
	} {
		cdRefused(t, name, data)
	}
	if err := checkSwapCalldata(cdSpec(t), mc.EVMTransaction{}, cdFloor); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("zero transaction: err = %v, want decode_failed", err)
	}
}

func TestCheckSwapCalldata_theApprovedAmountAndFloorAreExact(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Uint64Range(1, 1<<40).Draw(t, "amount")
		minOut := rapid.Uint64Range(1, 1<<40).Draw(t, "minOut")
		floor := rapid.Uint64Range(1, 1<<40).Draw(t, "floor")
		spec := app.EVMSwapSpec{
			Network:  chain.NetworkBase,
			Owner:    mustEVM(cdOwner),
			TokenIn:  mustEVM(guardUSDC),
			TokenOut: mustEVM(guardToken),
			AmountIn: amount,
		}
		data := evmcalls.Honest(cdOwner, guardUSDC, guardToken, amount, minOut)
		tx := mc.EVMTransaction{Network: chain.NetworkBase, To: mustEVM(guardRouter), Data: data, Value: new(big.Int)}
		if err := checkSwapCalldata(spec, tx, floor); (minOut >= floor) != (err == nil) {
			t.Fatalf("min out %d against floor %d: err = %v", minOut, floor, err)
		}
		spec.AmountIn = amount - 1
		if err := checkSwapCalldata(spec, tx, 1); err == nil {
			t.Fatalf("calldata spending %d accepted against an approval of %d", amount, amount-1)
		}
	})
}

func mustEVM(raw string) chain.EVMAddress {
	a, err := chain.ParseEVMAddress(raw)
	if err != nil {
		panic(err)
	}
	return a
}
