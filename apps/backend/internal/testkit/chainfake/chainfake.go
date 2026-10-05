package chainfake

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	Finality   = 13 * time.Second
	slotLength = 400 * time.Millisecond
)

type Wallets struct {
	testkit.Faults
	mu      sync.Mutex
	members map[privy.UserID]chain.Wallet
	apps    map[string]chain.Wallet
	creates int
}

func (w *Wallets) FindOrCreateMemberWallet(_ context.Context, user privy.UserID) (chain.Wallet, error) {
	if err := w.Check("FindOrCreateMemberWallet"); err != nil {
		return chain.Wallet{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if got, ok := w.members[user]; ok {
		return got, nil
	}
	wallet := newWallet("member:" + string(user))
	w.Seed(user, wallet)
	w.creates++
	return wallet, nil
}

func (w *Wallets) CreateAppWallet(_ context.Context, key string) (chain.Wallet, error) {
	if err := w.Check("CreateAppWallet"); err != nil {
		return chain.Wallet{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if got, ok := w.apps[key]; ok {
		return got, nil
	}
	if w.apps == nil {
		w.apps = map[string]chain.Wallet{}
	}
	w.apps[key] = newWallet("app:" + key)
	w.creates++
	return w.apps[key], nil
}

func (w *Wallets) Seed(user privy.UserID, wallet chain.Wallet) {
	if w.members == nil {
		w.members = map[privy.UserID]chain.Wallet{}
	}
	w.members[user] = wallet
}

func (w *Wallets) Creates() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.creates
}

func newWallet(label string) chain.Wallet {
	sum := sha256.Sum256([]byte(label))
	id := "wallet-" + hex.EncodeToString(sum[:6])
	return chain.Wallet{
		ID:           id,
		Address:      chain.AddressOf(fakes.PrivyWalletKey(id).Public().(ed25519.PublicKey)),
		HasAppSigner: true,
	}
}

type Signer struct {
	testkit.Faults
}

func (s *Signer) SignTransaction(_ context.Context, walletID string, unsigned []byte) ([]byte, error) {
	if err := s.Check("SignTransaction"); err != nil {
		return nil, err
	}
	tx, err := chain.DecodeTransaction(unsigned)
	if err != nil {
		return nil, err
	}
	if err := tx.Sign(fakes.PrivyWalletKey(walletID)); err != nil {
		return nil, err
	}
	return tx.Encode(), nil
}

type landing struct {
	at     time.Time
	failed bool
}

type Ledger struct {
	testkit.Faults
	clock  clock.Clock
	start  time.Time
	mu     sync.Mutex
	sol    map[chain.SolanaAddress]uint64
	tokens map[chain.SolanaAddress]map[chain.SolanaAddress]uint64
	landed map[chain.Signature]landing
}

func NewLedger(clk clock.Clock) *Ledger {
	return &Ledger{
		clock: clk, start: clk.Now(), sol: map[chain.SolanaAddress]uint64{},
		tokens: map[chain.SolanaAddress]map[chain.SolanaAddress]uint64{}, landed: map[chain.Signature]landing{},
	}
}

func (l *Ledger) SetSOL(addr chain.SolanaAddress, lamports uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sol[addr] = lamports
}

func (l *Ledger) SetTokens(owner chain.SolanaAddress, mint chain.Mint, amount uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tokens[owner] == nil {
		l.tokens[owner] = map[chain.SolanaAddress]uint64{}
	}
	l.tokens[owner][mint.Address] = amount
}

func (l *Ledger) Land(sig chain.Signature, failed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.landed[sig] = landing{at: l.clock.Now(), failed: failed}
}

func (l *Ledger) BlockHeight() uint64 {
	slots := int64(l.clock.Now().Sub(l.start) / slotLength)
	if slots < 0 {
		return 0
	}
	return uint64(slots)
}

func (l *Ledger) SOLBalance(_ context.Context, addr chain.SolanaAddress) (money.BaseUnits, error) {
	if err := l.Check("SOLBalance"); err != nil {
		return money.BaseUnits{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return money.NewBaseUnits(l.sol[addr], 9), nil
}

func (l *Ledger) TokenBalance(_ context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error) {
	if err := l.Check("TokenBalance"); err != nil {
		return money.BaseUnits{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return money.NewBaseUnits(l.tokens[owner][mint.Address], mint.Decimals), nil
}

func (l *Ledger) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	if err := l.Check("SignatureStatuses"); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now, height := l.clock.Now(), l.BlockHeight()
	out := make([]solana.Status, len(sigs))
	for i, sig := range sigs {
		out[i] = solana.Status{Signature: sig, State: solana.StateNotFound, BlockHeight: height}
		if got, ok := l.landed[sig]; ok {
			out[i].State, out[i].Failed = solana.StateProcessing, got.failed
			if now.Sub(got.at) >= Finality {
				out[i].State = solana.StateFinalized
			}
		}
	}
	return out, nil
}

func Relayer() *relayer.Relayer {
	key := chain.EncodeBase58(fakes.FixtureKey("chainfake-relayer"))
	r, err := relayer.New(config.Config{Relayer: config.Relayer{PrivateKey: key}}, nil)
	if err != nil {
		panic(err)
	}
	return r
}

func RelayerAddress() chain.SolanaAddress { return Relayer().Address() }

func WalletAddress(walletID string) chain.SolanaAddress {
	return chain.AddressOf(fakes.PrivyWalletKey(walletID).Public().(ed25519.PublicKey))
}

func Unsigned(signers ...chain.SolanaAddress) []byte {
	n := len(signers)
	msg := append(chain.CompactU16(n), 0, 0)
	msg = append(msg, chain.CompactU16(n)...)
	for _, s := range signers {
		key, _ := s.Bytes()
		msg = append(msg, key...)
	}
	msg = append(append(msg, make([]byte, ed25519.PublicKeySize)...), 0)
	return append(append(chain.CompactU16(n), make([]byte, n*ed25519.SignatureSize)...), msg...)
}
