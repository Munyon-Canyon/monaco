package domain

import (
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func EVMAsset(network chain.Network, address string) Asset {
	return Asset(string(network) + ":" + strings.ToLower(address))
}

func EVMCashAsset(network chain.Network) Asset { return EVMAsset(network, network.Cash().Address) }

func CashAssets(usdc Asset) []Asset {
	out := []Asset{usdc}
	for _, n := range chain.Networks() {
		if n.IsEVM() {
			out = append(out, EVMCashAsset(n))
		}
	}
	return out
}

func IsCash(asset, usdc Asset) bool { return slices.Contains(CashAssets(usdc), asset) }

type BridgeDirection string

const (
	BridgeOut  BridgeDirection = "bridge_out"
	BridgeBack BridgeDirection = "bridge_back"
)

type Bridge struct {
	SwapID    uuid.UUID
	CabalID   ids.CabalID
	Direction BridgeDirection
	Network   chain.Network
	Sent      money.Micros
	Fee       money.Micros
}

func (b Bridge) TxnID(status string) uuid.UUID {
	return uuid.NewSHA1(b.SwapID, []byte("bridge:"+string(b.Direction)+":"+status))
}

func (b Bridge) assets(usdc Asset) (source, destination Asset) {
	if b.Direction == BridgeBack {
		return EVMCashAsset(b.Network), usdc
	}
	return usdc, EVMCashAsset(b.Network)
}

func NewBridgeSentTxn(id uuid.UUID, b Bridge, usdc Asset) (CabalTxn, error) {
	source, _ := b.assets(usdc)
	return b.move(id, CabalTreasury, CabalVenue, source, b.Sent)
}

func NewBridgeLandedTxn(id uuid.UUID, b Bridge, usdc Asset) (CabalTxn, error) {
	landed, err := b.Sent.Sub(b.Fee)
	if err != nil {
		return CabalTxn{}, err
	}
	_, destination := b.assets(usdc)
	txn, err := b.move(id, CabalVenue, CabalTreasury, destination, landed)
	if err != nil || b.Fee.IsZero() {
		return txn, err
	}
	fee, err := b.move(id, CabalVenue, CabalFees, destination, b.Fee)
	if err != nil {
		return CabalTxn{}, err
	}
	return NewCabalTxn(txn.CabalTxnHeader, append(txn.Entries(), fee.Entries()...))
}

func NewBridgeReturnedTxn(id uuid.UUID, b Bridge, usdc Asset) (CabalTxn, error) {
	source, _ := b.assets(usdc)
	return b.move(id, CabalVenue, CabalTreasury, source, b.Sent)
}

func (b Bridge) move(id uuid.UUID, from, to CabalAccount, asset Asset, amount money.Micros) (CabalTxn, error) {
	debit, err := money.Micros{}.Delta(amount)
	if err != nil {
		return CabalTxn{}, err
	}
	credit, err := amount.Delta(money.Micros{})
	if err != nil {
		return CabalTxn{}, err
	}
	return NewCabalTxn(CabalTxnHeader{ID: id, CabalID: b.CabalID, Kind: CabalSwap, Status: TxnSettled}, []CabalEntry{
		{Account: from, Asset: asset, Amount: debit},
		{Account: to, Asset: asset, Amount: credit},
	})
}
