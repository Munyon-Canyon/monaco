package relayer_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	recordedOrders = "testdata/ultra_orders.json"
	recordedPayer  = chain.SolanaAddress("6zPvHbTHwVakhN3FiVmFqjUFhY5mdAAt865vUAZFFyVr")
)

type recordedOrder struct {
	Name           string `json:"name"`
	Router         string `json:"router"`
	InputMint      string `json:"input_mint"`
	OutputMint     string `json:"output_mint"`
	Amount         string `json:"amount"`
	ExcludeRouters string `json:"exclude_routers"`
	Transaction    string `json:"transaction"`
}

func loadOrders(t *testing.T) []recordedOrder {
	t.Helper()
	raw, err := os.ReadFile(recordedOrders)
	if err != nil {
		t.Fatal(err)
	}
	var orders []recordedOrder
	if err := json.Unmarshal(raw, &orders); err != nil {
		t.Fatal(err)
	}
	if len(orders) == 0 {
		t.Fatal("no recorded orders")
	}
	return orders
}

func (o recordedOrder) raw(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(o.Transaction)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (o recordedOrder) paidByTheRelayer(t *testing.T) bool {
	t.Helper()
	return decodeMessage(t, o.raw(t)).keys[0] == recordedPayer
}

func (o recordedOrder) asFixtureRelayer(t *testing.T) txMessage {
	t.Helper()
	m := decodeMessage(t, o.raw(t))
	m.keys[0] = chain.AddressOf(keyOf("relayer"))
	return m
}

func relayerOrders(t *testing.T) []recordedOrder {
	t.Helper()
	var out []recordedOrder
	for _, o := range loadOrders(t) {
		if o.paidByTheRelayer(t) {
			out = append(out, o)
		}
	}
	return out
}

func recordedRelayer(t *testing.T) *relayer.Relayer {
	t.Helper()
	cfg := keyConfig("relayer")
	r, err := relayer.New(cfg, solana.New(cfg, clock.Real{}))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCoSign_signsEveryRecordedUltraTransactionTheRelayerPays(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	orders := relayerOrders(t)
	if len(orders) < 10 {
		t.Fatalf("%d recorded orders paid by the relayer, want at least 10", len(orders))
	}
	for _, o := range orders {
		t.Run(o.Name, func(t *testing.T) {
			t.Parallel()
			m := o.asFixtureRelayer(t)
			signed, err := r.CoSign(t.Context(), m.encode())
			if err != nil {
				t.Fatalf("CoSign = %v, want a signed transaction", err)
			}
			before, _ := chain.DecodeTransaction(m.encode())
			after, err := chain.DecodeTransaction(signed)
			if err != nil || !after.Signed(0) || !bytes.Equal(before.Message, after.Message) {
				t.Fatalf("signed = %v, %v; want the same message signed by the fee payer", after.Signed(0), err)
			}
		})
	}
}

func TestRecordedUltraTransactions_coverEveryRoleTheRelayerPlays(t *testing.T) {
	t.Parallel()
	roles := map[chain.SolanaAddress]string{
		chain.ATAProgram:         "funds an account",
		chain.SystemProgram:      "receives a transfer",
		relayer.JupiterV6Program: "is named in the route",
	}
	seen := map[string]bool{}
	for _, o := range relayerOrders(t) {
		m := decodeMessage(t, o.raw(t))
		for _, ix := range m.instructions {
			if !slices.Contains(ix.accounts, 0) {
				continue
			}
			if role, ok := roles[m.keys[ix.program]]; ok {
				seen[role] = true
			}

		}
	}
	for _, want := range roles {
		if !seen[want] {
			t.Errorf("no recorded transaction where the relayer %s", want)
		}
	}
}

func TestCoSign_refusesTheMarketMakerRoutesAsJupiterPaysTheirFee(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	var seen int
	for _, o := range loadOrders(t) {
		if o.Router != "jupiterz" {
			continue
		}
		seen++
		if o.paidByTheRelayer(t) {
			t.Fatalf("%s: the market maker accepted the relayer as payer; record it as a signable route", o.Name)
		}
		signed, err := r.CoSign(t.Context(), o.raw(t))
		wantCode(t, err, errs.CodeInvalidInput)
		if signed != nil {
			t.Fatal("a refused transaction came back signed")
		}
		tx, err := chain.DecodeTransaction(o.asFixtureRelayer(t).encode())
		if err != nil {
			t.Fatal(err)
		}
		_, err = relayer.Inspect(relayer.DefaultLimits(), tx.Message)
		wantCode(t, err, errs.CodeInvalidInput)
	}
	if seen == 0 {
		t.Fatal("no recorded market maker orders")
	}
}

func TestCoSign_refusesATamperedCopyOfEveryRecordedUltraTransaction(t *testing.T) {
	t.Parallel()
	r := recordedRelayer(t)
	wallet := chain.AddressOf(keyOf("treasury"))
	tampers := map[string]func(m txMessage) txMessage{
		"a system transfer out of the relayer": func(m txMessage) txMessage {
			m = m.clone()
			system := m.keyIndex(chain.SystemProgram)
			to := m.keyIndex(wallet)
			amount := binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 1_000_000_000)
			return m.with(rawIx{program: system, accounts: []int{0, to}, data: amount})
		},
		"a priority fee raised past the cap": func(m txMessage) txMessage {
			return m.mapData(relayer.ComputeBudgetProgram, func(data []byte) []byte {
				if data[0] != 3 {
					return data
				}
				binary.LittleEndian.PutUint64(data[1:], 1<<40)
				return data
			})
		},
		"a priority fee of twice the cap": func(m txMessage) txMessage {
			return m.mapData(relayer.ComputeBudgetProgram, func(data []byte) []byte {
				switch data[0] {
				case 2:
					binary.LittleEndian.PutUint32(data[1:], 1_000_000)
				case 3:
					binary.LittleEndian.PutUint64(data[1:], 2*relayer.DefaultMaxFeeLamports)
				}
				return data
			})
		},
		"a third account creation funded by the relayer": func(m txMessage) txMessage {
			m = m.clone()
			ata := m.keyIndex(chain.ATAProgram)
			system := m.keyIndex(chain.SystemProgram)
			token := m.keyIndex(chain.SPLProgram)
			create := func(n int) rawIx {
				address := m.keyIndex(chain.AddressOf(keyOf("new-ata-" + strconv.Itoa(n))))
				return rawIx{
					program:  ata,
					accounts: []int{0, address, address, address, system, token},
					data:     []byte{1},
				}
			}
			return m.with(create(0), create(1), create(2))
		},
		"the relayer as the owner of a token transfer": func(m txMessage) txMessage {
			m = m.clone()
			token := m.keyIndex(chain.SPLProgram)
			source := m.keyIndex(chain.AddressOf(keyOf("source")))
			return m.with(
				rawIx{program: token, accounts: []int{source, source, 0}, data: []byte{3, 1, 0, 0, 0, 0, 0, 0, 0}},
			)
		},
		"an account creation funded by the relayer": func(m txMessage) txMessage {
			m = m.clone()
			system := m.keyIndex(chain.SystemProgram)
			created := m.keyIndex(chain.AddressOf(keyOf("created")))
			data := binary.LittleEndian.AppendUint32(nil, 0)
			data = binary.LittleEndian.AppendUint64(data, 1_000_000_000)
			data = binary.LittleEndian.AppendUint64(data, 0)
			data = append(data, make([]byte, 32)...)
			return m.with(rawIx{program: system, accounts: []int{0, created}, data: data})
		},
		"the relayer handed to a program no swap uses": func(m txMessage) txMessage {
			m = m.clone()
			stranger := m.keyIndex(chain.AddressOf(keyOf("stranger-program")))
			return m.with(rawIx{program: stranger, accounts: []int{0}, data: []byte{1}})
		},
	}
	for _, o := range relayerOrders(t) {
		for name, tamper := range tampers {
			t.Run(o.Name+"/"+name, func(t *testing.T) {
				t.Parallel()
				tampered := tamper(o.asFixtureRelayer(t)).encode()
				signed, err := r.CoSign(t.Context(), tampered)
				wantCode(t, err, errs.CodeInvalidInput)
				if signed != nil {
					t.Fatal("a refused transaction came back signed")
				}
			})
		}
	}
}

func keyOf(label string) []byte { return fakes.FixtureKey(label).Public().(ed25519.PublicKey) }
