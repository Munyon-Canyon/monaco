package chain_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

const (
	recordedOrders = "../../../../platform/chain/relayer/testdata/ultra_orders.json"
	recordedPayer  = platform.SolanaAddress("6zPvHbTHwVakhN3FiVmFqjUFhY5mdAAt865vUAZFFyVr")
)

type recordedOrder struct {
	Name        string `json:"name"`
	Router      string `json:"router"`
	Transaction string `json:"transaction"`
}

func recorded(t *testing.T) []recordedOrder {
	t.Helper()
	raw, err := os.ReadFile(recordedOrders)
	if err != nil {
		t.Fatal(err)
	}
	var orders []recordedOrder
	if err := json.Unmarshal(raw, &orders); err != nil {
		t.Fatal(err)
	}
	return orders
}

func (o recordedOrder) forTheFixtures(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(o.Transaction)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := platform.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	swap := func(from, to platform.SolanaAddress) {
		a, _ := from.Bytes()
		b, _ := to.Bytes()
		raw = bytes.Replace(raw, a, b, 1)
	}
	swap(tx.Signers[1], chainfake.WalletAddress("wallet-a"))
	swap(recordedPayer, chainfake.RelayerAddress())
	return raw
}

func relayerSlotOf(tx platform.Transaction) int {
	return slices.Index(tx.Signers, chainfake.RelayerAddress())
}

func assertSignedSlots(t *testing.T, tx platform.Transaction) {
	t.Helper()
	relayerSlot := relayerSlotOf(tx)
	for i := range tx.Signatures {
		if i == 0 && relayerSlot == 0 {
			continue
		}
		if want := i == 1 || i == relayerSlot; tx.Signed(i) != want {
			t.Fatalf("slot %d signed = %v, want %v (relayer at slot %d)", i, tx.Signed(i), want, relayerSlot)
		}
	}
}

func TestSigner_signsEveryRecordedUltraTransactionTheWayItsRouterNeedsIt(t *testing.T) {
	t.Parallel()
	s := chain.NewSigner(&chainfake.Signer{}, chainfake.Relayer())
	orders := recorded(t)
	if !slices.ContainsFunc(orders, func(o recordedOrder) bool { return o.Router == "jupiterz" }) ||
		!slices.ContainsFunc(orders, func(o recordedOrder) bool { return o.Router != "jupiterz" }) {
		t.Fatal("want recorded orders from both routers")
	}
	for _, o := range orders {
		t.Run(o.Name+"/"+o.Router, func(t *testing.T) {
			t.Parallel()
			signed, sig, err := s.Sign(t.Context(), "wallet-a", o.forTheFixtures(t))
			if err != nil {
				t.Fatalf("Sign = %v, want a signed transaction", err)
			}
			tx, err := platform.DecodeTransaction(signed)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedSlots(t, tx)
			want := platform.StandInSignature(platform.SignatureOf(tx.Signatures[1]))
			if relayerSlotOf(tx) == 0 {
				want = platform.SignatureOf(tx.Signatures[0])
			}
			if sig != want {
				t.Fatalf("signature = %q, want %q", sig, want)
			}
		})
	}
}
