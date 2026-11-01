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
)

const (
	jupiterV6 = chain.SolanaAddress("JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4")
	cbProgram = relayer.ComputeBudgetProgram
)

func baseline(t *testing.T) txMessage {
	t.Helper()
	m := txMessage{
		signatures: 2,
		versioned:  true,
		header:     [headerSize]byte{2, 0, 0},
		blockhash:  make([]byte, blockhashSize),
		keys: []chain.SolanaAddress{
			chain.AddressOf(keyOf("relayer")), chain.AddressOf(keyOf("treasury")),
		},
		tail: []byte{0},
	}
	return m
}

func (m txMessage) program(addr chain.SolanaAddress, accounts []int, data []byte) txMessage {
	out := m.clone()
	index := out.keyIndex(addr)
	for i, a := range accounts {
		if a < 0 {
			accounts[i] = out.keyIndex(chain.AddressOf(keyOf("account-" + string(rune('a'-a-1)))))
		}
	}
	out.instructions = append(out.instructions, rawIx{program: index, accounts: accounts, data: data})
	return out
}

func budget(discriminator byte, value uint64, width int) []byte {
	data := binary.LittleEndian.AppendUint64(nil, value)
	return append([]byte{discriminator}, data[:width]...)
}

func (m txMessage) unitLimit(v uint64) txMessage { return m.program(cbProgram, nil, budget(2, v, 4)) }

func (m txMessage) unitPrice(v uint64) txMessage { return m.program(cbProgram, nil, budget(3, v, 8)) }

func (m txMessage) createAccount(funder int) txMessage {
	return m.program(chain.ATAProgram, []int{funder, -1, 1, -2, 1, 1}, []byte{1})
}

func (m txMessage) swap(program chain.SolanaAddress, accounts ...int) txMessage {
	return m.program(program, accounts, []byte{9, 9, 9})
}

func (m txMessage) transfer(from, to int) txMessage {
	return m.program(
		chain.SystemProgram,
		[]int{from, to},
		binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 1_000_000_000),
	)
}

func TestCoSign_signsOnlyTheRelayersExpectedRole(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	r, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	b := baseline(t)
	cases := []struct {
		name string
		m    txMessage
		want errs.Code
	}{
		{"a swap that never touches the relayer", b.swap(jupiterV6, 1), ""},
		{"a route that names the relayer", b.swap(jupiterV6, 1, 0), ""},
		{"a market maker fill that names the relayer", b.swap(relayer.JupiterZProgram, 1, 0), errs.CodeInvalidInput},
		{"a system transfer into the relayer", b.swap(jupiterV6, 1).transfer(1, 0), ""},
		{"a system transfer from the relayer to itself", b.swap(jupiterV6, 1).transfer(0, 0), errs.CodeInvalidInput},
		{"a modest priority fee", b.unitLimit(200_000).unitPrice(5_000).swap(jupiterV6, 1), ""},
		{"a priority fee exactly at the cap", b.unitLimit(1_000_000).unitPrice(1_000_000).swap(jupiterV6, 1), ""},
		{
			"a priority fee one lamport over the cap", b.unitLimit(1_000_001).unitPrice(1_000_000).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{
			"a price with no limit is costed at the maximum limit",
			b.unitPrice(800_000_000).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{
			"an enormous price cannot overflow the check", b.unitLimit(1<<32-1).unitPrice(1<<63).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{"a loaded accounts data size limit", b.program(cbProgram, nil, budget(4, 64_000, 4)).swap(jupiterV6, 1), ""},
		{
			"a loaded accounts data size limit with a short payload",
			b.program(cbProgram, nil, budget(4, 64_000, 2)).swap(jupiterV6, 1), errs.CodeInvalidInput,
		},
		{
			"a compute budget instruction it cannot read",
			b.program(cbProgram, nil, []byte{1, 0}).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{
			"an empty compute budget instruction",
			b.program(cbProgram, nil, nil).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{"one account funded by the relayer", b.createAccount(0).swap(jupiterV6, 1), ""},
		{"two accounts funded by the relayer", b.createAccount(0).createAccount(0).swap(jupiterV6, 1), ""},
		{
			"three accounts funded by the relayer",
			b.createAccount(0).createAccount(0).createAccount(0).swap(jupiterV6, 1),
			errs.CodeInvalidInput,
		},
		{
			"three accounts funded by the taker",
			b.createAccount(1).createAccount(1).createAccount(1).swap(jupiterV6, 1),
			"",
		},
		{
			"an account creation that is not the idempotent one",
			b.program(chain.ATAProgram, []int{0, -1, 1, -2, 1, 1}, nil).swap(jupiterV6, 1), errs.CodeInvalidInput,
		},
		{
			"an account created for the relayer itself",
			b.program(chain.ATAProgram, []int{0, -1, 0, -2, 1, 1}, []byte{1}).swap(jupiterV6, 1), errs.CodeInvalidInput,
		},
		{
			"an account creation with too few accounts",
			b.program(chain.ATAProgram, []int{0, -1, 1}, []byte{1}).swap(jupiterV6, 1), errs.CodeInvalidInput,
		},
		{"a system transfer out of the relayer", b.swap(jupiterV6, 1).transfer(0, 1), errs.CodeInvalidInput},
		{"a system transfer between other accounts", b.swap(jupiterV6, 1).transfer(1, 1), ""},
		{
			"a token instruction that names the relayer",
			b.swap(jupiterV6, 1).program(chain.SPLProgram, []int{-1, -2, 0}, []byte{3}),
			errs.CodeInvalidInput,
		},
		{
			"a program no swap uses that takes the relayer",
			b.swap(chain.AddressOf(keyOf("stranger")), 0),
			errs.CodeInvalidInput,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			signed, err := r.CoSign(t.Context(), c.m.encode())
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

func TestInspect_pricesTheFeeAndTheRentTheRelayerPays(t *testing.T) {
	t.Parallel()
	m := baseline(t).unitLimit(200_000).unitPrice(5_000).createAccount(0).createAccount(0).swap(jupiterV6, 1)
	limits := relayer.DefaultLimits()
	got, err := relayer.Inspect(limits, m.encode()[1+2*ed25519.SignatureSize:])
	if err != nil {
		t.Fatal(err)
	}
	want := relayer.Spend{
		PriorityFeeLamports: 1_000, BaseFeeLamports: 10_000, ATACreations: 2, RentLamports: 2 * limits.ATARentLamports,
	}
	if got != want {
		t.Fatalf("spend = %+v, want %+v", got, want)
	}
}

func TestCoSign_theCapsComeFromTheLimitsItWasBuiltWith(t *testing.T) {
	t.Parallel()
	cfg := keyConfig("relayer")
	tight := relayer.DefaultLimits()
	tight.MaxFeeLamports = 100
	tight.MaxATACreations = 0
	r, _ := relayer.New(cfg, solana.New(cfg, clock.Real{}), relayer.WithLimits(tight))
	b := baseline(t)
	if _, err := r.CoSign(
		t.Context(),
		b.unitLimit(200_000).unitPrice(5_000).swap(jupiterV6, 1).encode(),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("a 1,000 lamport fee under a 100 lamport cap = %v, want a refusal", err)
	}
	if _, err := r.CoSign(
		t.Context(),
		b.createAccount(0).swap(jupiterV6, 1).encode(),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("an account creation under a cap of none = %v, want a refusal", err)
	}
	if _, err := r.CoSign(t.Context(), b.swap(jupiterV6, 1).encode()); err != nil {
		t.Fatalf("a free swap = %v, want it signed", err)
	}
}

func TestInspect_refusesAnyTruncationOfAValidMessage(t *testing.T) {
	t.Parallel()
	for _, versioned := range []bool{false, true} {
		m := baseline(t).unitLimit(200_000).unitPrice(5_000).createAccount(0).swap(jupiterV6, 1, 0)
		m.versioned = versioned
		if !versioned {
			m.tail = nil
		}
		raw := m.encode()
		msg := raw[1+2*ed25519.SignatureSize:]
		if _, err := relayer.Inspect(relayer.DefaultLimits(), msg); err != nil {
			t.Fatalf("whole message: %v", err)
		}
		last := len(msg)
		if versioned {
			last--
		}
		for n := range last {
			_, err := relayer.Inspect(relayer.DefaultLimits(), msg[:n])
			wantCode(t, err, errs.CodeInvalidInput)
		}
	}
}

func TestInspect_refusesMalformedStructure(t *testing.T) {
	t.Parallel()
	valid := baseline(t).swap(jupiterV6, 1).encode()[1+2*ed25519.SignatureSize:]
	keysEnd := 1 + headerSize + 1 + 32*3 + 32
	head := valid[:keysEnd]
	cases := map[string][]byte{
		"a length prefix of four bytes":  append(append([]byte(nil), head...), 0x80, 0x80, 0x80, 0x80),
		"a program index past the keys":  append(append([]byte(nil), head...), 1, 9, 0, 0),
		"more accounts than bytes":       append(append([]byte(nil), head...), 1, 1, 50, 0),
		"more data than bytes":           append(append([]byte(nil), head...), 1, 1, 1, 0, 50, 7),
		"more keys than the message has": append([]byte{2, 0, 1, 90}, make([]byte, 40)...),
		"no signers in the header":       append([]byte{0, 0, 0}, valid[1+headerSize:]...),
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := relayer.Inspect(relayer.DefaultLimits(), msg)
			wantCode(t, err, errs.CodeInvalidInput)
		})
	}
}
