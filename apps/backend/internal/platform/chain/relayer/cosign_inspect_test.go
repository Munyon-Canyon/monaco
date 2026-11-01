package relayer_test

import (
	"crypto/ed25519"
	"encoding/binary"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type ix struct {
	program chain.SolanaAddress
	data    []byte
}

func budget(discriminator byte, value uint64, width int) ix {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, value)
	return ix{relayer.ComputeBudgetProgram, append([]byte{discriminator}, data[:width]...)}
}

func unitLimit(v uint64) ix { return budget(2, v, 4) }

func unitPrice(v uint64) ix { return budget(3, v, 8) }

func swapIx() ix { return ix{relayer.JupiterV6Program, []byte{9, 9, 9}} }

func buildSwap(versioned bool, instructions ...ix) []byte {
	payer := fakes.FixtureKey("relayer").Public().(ed25519.PublicKey)
	wallet := fakes.FixtureKey("other").Public().(ed25519.PublicKey)
	keys := []chain.SolanaAddress{chain.AddressOf(payer), chain.AddressOf(wallet)}
	index := map[chain.SolanaAddress]byte{}
	const ordinary = 2
	for _, in := range instructions {
		if _, ok := index[in.program]; !ok {
			index[in.program] = byte(len(keys) & 0xff)
			keys = append(keys, in.program)
		}
	}
	msg := []byte{}
	if versioned {
		msg = append(msg, 0x80)
	}
	msg = append(msg, 2, 0, byte((len(keys)-ordinary)&0xff))
	msg = append(msg, chain.CompactU16(len(keys))...)
	for _, k := range keys {
		raw, _ := k.Bytes()
		msg = append(msg, raw...)
	}
	msg = append(msg, make([]byte, 32)...)
	msg = append(msg, chain.CompactU16(len(instructions))...)
	for _, in := range instructions {
		msg = append(msg, index[in.program], 1, 1)
		msg = append(msg, chain.CompactU16(len(in.data))...)
		msg = append(msg, in.data...)
	}
	if versioned {
		msg = append(msg, 0)
	}
	return append(append(chain.CompactU16(2), make([]byte, 128)...), msg...)
}

func TestCoSign_signsOnlyTheExpectedSwapShape(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	r, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	ata := ix{chain.ATAProgram, []byte{1}}
	transfer := ix{chain.SystemProgram, []byte{2, 0, 0, 0, 0, 0xca, 0x9a, 0x3b, 0, 0, 0, 0}}
	cases := []struct {
		name string
		tx   []byte
		want errs.Code
	}{
		{"swap with a modest priority fee", buildSwap(true, unitLimit(200_000), unitPrice(5_000), swapIx()), ""},
		{"legacy message with a swap", buildSwap(false, swapIx()), ""},
		{"swap that creates one account", buildSwap(true, unitLimit(200_000), ata, swapIx()), ""},
		{"priority fee exactly at the cap", buildSwap(true, unitLimit(1_000_000), unitPrice(1_000_000), swapIx()), ""},
		{
			"priority fee one lamport over the cap",
			buildSwap(true, unitLimit(1_000_001), unitPrice(1_000_000), swapIx()),
			errs.CodeInvalidInput,
		},
		{
			"price with no limit is costed at the maximum limit", buildSwap(true, unitPrice(800_000_000), swapIx()),
			errs.CodeInvalidInput,
		},
		{
			"an enormous price cannot overflow the check",
			buildSwap(true, unitLimit(1<<32-1), unitPrice(1<<63), swapIx()),
			errs.CodeInvalidInput,
		},
		{"a system transfer", buildSwap(true, swapIx(), transfer), errs.CodeInvalidInput},
		{"a program that is not the swap", buildSwap(true, ix{chain.SPLProgram, []byte{3}}), errs.CodeInvalidInput},
		{"three account creations", buildSwap(true, ata, ata, ata, swapIx()), errs.CodeInvalidInput},
		{
			"an ata instruction that is not a create", buildSwap(true, ix{chain.ATAProgram, []byte{2}}, swapIx()),
			errs.CodeInvalidInput,
		},
		{"a compute budget instruction it cannot read", buildSwap(true, ix{relayer.ComputeBudgetProgram, []byte{1, 0}},
			swapIx()), errs.CodeInvalidInput},
		{
			"an empty compute budget instruction", buildSwap(true, ix{relayer.ComputeBudgetProgram, nil}, swapIx()),
			errs.CodeInvalidInput,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			signed, err := r.CoSign(t.Context(), c.tx)
			if c.want == "" {
				if err != nil {
					t.Fatalf("CoSign = %v, want a signed transaction", err)
				}
				tx, _ := chain.DecodeTransaction(signed)
				if !tx.Signed(0) {
					t.Fatal("the fee payer did not sign")
				}
				return
			}
			wantCode(t, err, c.want)
			if signed != nil {
				t.Fatal("a refused transaction came back signed")
			}
		})
	}
}
