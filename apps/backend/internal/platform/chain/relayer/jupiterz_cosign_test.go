package relayer_test

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
)

func fixtureRelayerAddress() chain.SolanaAddress { return chain.AddressOf(keyOf("relayer")) }

func (o recordedOrder) withFixtureRelayer(t *testing.T) txMessage {
	t.Helper()
	m := decodeMessage(t, o.raw(t))
	for i, k := range m.keys {
		if k == recordedPayer {
			m.keys[i] = fixtureRelayerAddress()
		}
	}
	return m
}

func (m txMessage) relayerSigner() int {
	i := slices.Index(m.keys, fixtureRelayerAddress())
	if i < 0 || i >= int(m.header[0]) {
		return -1
	}
	return i
}

func marketMakerOrders(t *testing.T, namingTheRelayer bool) []recordedOrder {
	t.Helper()
	var out []recordedOrder
	for _, o := range loadOrders(t) {
		if o.Router == "jupiterz" && (o.withFixtureRelayer(t).relayerSigner() > 0) == namingTheRelayer {
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no recorded market maker orders where the relayer signer is %v", namingTheRelayer)
	}
	return out
}

func assertOnlyTheRelayerSigned(t *testing.T, signed []byte, m txMessage) {
	t.Helper()
	slot := m.relayerSigner()
	before, _ := chain.DecodeTransaction(m.encode())
	after, err := chain.DecodeTransaction(signed)
	if err != nil || !bytes.Equal(before.Message, after.Message) {
		t.Fatalf("signed = %v; want the same message", err)
	}
	for i := range after.Signatures {
		if after.Signed(i) != (i == slot) {
			t.Fatalf("slot %d signed = %v, want only the relayer's slot %d", i, after.Signed(i), slot)
		}
	}
}

func TestCoSign_signsTheRecordedMarketMakerRoutesThatNameTheRelayerAsFunder(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	for _, o := range marketMakerOrders(t, true) {
		t.Run(o.Name, func(t *testing.T) {
			t.Parallel()
			m := o.withFixtureRelayer(t)
			signed, err := r.CoSign(t.Context(), m.encode())
			if err != nil {
				t.Fatalf("CoSign = %v, want a signed transaction", err)
			}
			assertOnlyTheRelayerSigned(t, signed, m)
		})
	}
}

func TestCoSign_refusesTheRecordedMarketMakerRoutesThatDoNotNameTheRelayer(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	for _, o := range marketMakerOrders(t, false) {
		t.Run(o.Name, func(t *testing.T) {
			t.Parallel()
			signed, err := r.CoSign(t.Context(), o.withFixtureRelayer(t).encode())
			wantCode(t, err, errs.CodeInvalidInput)
			if signed != nil {
				t.Fatal("a refused transaction came back signed")
			}
		})
	}
}

func TestRecordedMarketMakerOrders_includeOneThatNamesTheRelayerAsAFunder(t *testing.T) {
	t.Parallel()
	for _, o := range marketMakerOrders(t, true) {
		m := o.withFixtureRelayer(t)
		slot := m.relayerSigner()
		funds := slices.ContainsFunc(m.instructions, func(ix rawIx) bool {
			return m.keys[ix.program] == chain.ATAProgram && len(ix.accounts) > 0 && ix.accounts[0] == slot
		})
		if !funds {
			t.Fatalf("%s: the relayer signs but funds no account", o.Name)
		}
	}
}

func TestCoSign_refusesATamperedCopyOfEveryRecordedMarketMakerRouteThatNamesTheRelayer(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	tampers := map[string]func(m txMessage, relayerAt int) txMessage{
		"a system transfer out of the relayer": func(m txMessage, at int) txMessage {
			m = m.clone()
			system := m.keyIndex(chain.SystemProgram)
			to := m.keyIndex(chain.AddressOf(keyOf("thief")))
			amount := binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 1_000_000_000)
			return m.with(rawIx{program: system, accounts: []int{at, to}, data: amount})
		},
		"a third account creation funded by the relayer": func(m txMessage, at int) txMessage {
			m = m.clone()
			ata := m.keyIndex(chain.ATAProgram)
			system := m.keyIndex(chain.SystemProgram)
			token := m.keyIndex(chain.SPLProgram)
			create := func(n int) rawIx {
				address := m.keyIndex(chain.AddressOf(keyOf("new-ata-" + strconv.Itoa(n))))
				return rawIx{
					program:  ata,
					accounts: []int{at, address, address, address, system, token},
					data:     []byte{1},
				}
			}
			return m.with(create(0), create(1), create(2))
		},
		"the relayer as the owner of a token transfer": func(m txMessage, at int) txMessage {
			m = m.clone()
			token := m.keyIndex(chain.SPLProgram)
			source := m.keyIndex(chain.AddressOf(keyOf("source")))
			return m.with(rawIx{
				program: token, accounts: []int{source, source, at}, data: []byte{3, 1, 0, 0, 0, 0, 0, 0, 0},
			})
		},
		"an account creation funded by the relayer": func(m txMessage, at int) txMessage {
			m = m.clone()
			system := m.keyIndex(chain.SystemProgram)
			created := m.keyIndex(chain.AddressOf(keyOf("created")))
			data := binary.LittleEndian.AppendUint32(nil, 0)
			data = binary.LittleEndian.AppendUint64(data, 1_000_000_000)
			data = binary.LittleEndian.AppendUint64(data, 0)
			data = append(data, make([]byte, 32)...)
			return m.with(rawIx{program: system, accounts: []int{at, created}, data: data})
		},
		"the relayer handed to the market maker program": func(m txMessage, at int) txMessage {
			m = m.clone()
			program := m.keyIndex(relayer.JupiterZProgram)
			return m.with(rawIx{program: program, accounts: []int{1, at}, data: []byte{1}})
		},
		"the relayer handed to a program no swap uses": func(m txMessage, at int) txMessage {
			m = m.clone()
			stranger := m.keyIndex(chain.AddressOf(keyOf("stranger-program")))
			return m.with(rawIx{program: stranger, accounts: []int{at}, data: []byte{1}})
		},
	}
	for _, o := range marketMakerOrders(t, true) {
		for name, tamper := range tampers {
			t.Run(o.Name+"/"+name, func(t *testing.T) {
				t.Parallel()
				m := o.withFixtureRelayer(t)
				signed, err := r.CoSign(t.Context(), tamper(m, m.relayerSigner()).encode())
				wantCode(t, err, errs.CodeInvalidInput)
				if signed != nil {
					t.Fatal("a refused transaction came back signed")
				}
			})
		}
	}
}

func TestInspect_aRelayerThatDoesNotPayTheFeeIsNotChargedForIt(t *testing.T) {
	t.Parallel()
	b := baseline(t)
	b.header = [headerSize]byte{3, 0, 0}
	b.signatures = 3
	b.keys = []chain.SolanaAddress{
		chain.AddressOf(keyOf("market-maker")), chain.AddressOf(keyOf("treasury")), chain.AddressOf(keyOf("relayer")),
	}
	m := b.unitLimit(1_400_000).unitPrice(900_000_000).createAccount(2).swap(relayer.JupiterZProgram, 1, 0)
	msg := m.encode()[1+3*64:]
	got, err := relayer.InspectAt(relayer.DefaultLimits(), msg, 2)
	if err != nil {
		t.Fatalf("a priority fee the market maker pays = %v, want it ignored", err)
	}
	want := relayer.Spend{ATACreations: 1, RentLamports: relayer.DefaultLimits().ATARentLamports}
	if got != want {
		t.Fatalf("spend = %+v, want %+v", got, want)
	}
	if _, err := relayer.InspectAt(relayer.DefaultLimits(), msg, 0); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("the same fee with the relayer as payer = %v, want refused", err)
	}
}

func TestInspect_theRelayerAsAFunderStillHasAnAccountCap(t *testing.T) {
	t.Parallel()
	b := baseline(t)
	b.header = [headerSize]byte{3, 0, 0}
	b.signatures = 3
	b.keys = []chain.SolanaAddress{
		chain.AddressOf(keyOf("market-maker")), chain.AddressOf(keyOf("treasury")), chain.AddressOf(keyOf("relayer")),
	}
	m := b.createAccount(2).createAccount(2).createAccount(2).swap(relayer.JupiterZProgram, 1)
	_, err := relayer.InspectAt(relayer.DefaultLimits(), m.encode()[1+3*64:], 2)
	wantCode(t, err, errs.CodeInvalidInput)
}
