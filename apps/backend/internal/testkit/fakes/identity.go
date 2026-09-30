package fakes

import (
	"bytes"
	"cmp"
	"context"
	"encoding/hex"
	"log/slog"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type Identity struct {
	testkit.Faults
	mu      sync.Mutex
	cards   []identity.UserCard
	wallets []identity.MemberWallet
	phones  map[string]ids.UserID
	xUsers  map[string]ids.UserID
}

var _ identity.Queries = (*Identity)(nil)

func NewIdentity(cards []identity.UserCard, wallets []identity.MemberWallet) *Identity {
	f := &Identity{
		cards:   make([]identity.UserCard, len(cards)),
		wallets: slices.Clone(wallets),
		phones:  map[string]ids.UserID{},
		xUsers:  map[string]ids.UserID{},
	}
	for i, card := range cards {
		f.cards[i] = shownCard(card)
	}
	slices.SortFunc(f.wallets, func(a, b identity.MemberWallet) int { return compareUserIDs(a.UserID, b.UserID) })
	return f
}

func shownCard(card identity.UserCard) identity.UserCard {
	if card.Deleted {
		card.Handle, card.DisplayName, card.PhotoURL = "", "", ""
		return card
	}
	card.DisplayName = cmp.Or(card.DisplayName, card.Handle)
	return card
}

func (f *Identity) SetPhoneHash(id ids.UserID, hash []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.phones[hex.EncodeToString(hash)] = id
}

func (f *Identity) SetXUserID(id ids.UserID, xUserID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.xUsers[xUserID] = id
}

func (f *Identity) UsersByID(_ context.Context, userIDs []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	if err := f.begin("UsersByID", len(userIDs), 0, identity.MaxUsersByID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := make(map[ids.UserID]identity.UserCard, len(userIDs))
	for _, id := range userIDs {
		if card, ok := f.card(id); ok {
			found[id] = card
		}
	}
	return found, nil
}

func (f *Identity) UserByHandle(_ context.Context, handle string) (identity.UserCard, error) {
	const op = "UserByHandle"
	key, ok := identityHandleKey(handle)
	if !ok {
		return identity.UserCard{}, errs.New(errs.CodeUserNotFound, "fakes.Identity."+op)
	}
	if err := f.Check(op); err != nil {
		return identity.UserCard{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := f.liveHandle(key); i >= 0 {
		return f.cards[i], nil
	}
	return identity.UserCard{}, errs.New(errs.CodeUserNotFound, "fakes.Identity."+op)
}

func (f *Identity) UserIDsByHandles(_ context.Context, handles []string) (map[string]ids.UserID, error) {
	if err := f.begin("UserIDsByHandles", len(handles), 0, identity.MaxHandles); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := make(map[string]ids.UserID, len(handles))
	for _, h := range handles {
		if key, ok := identityHandleKey(h); ok {
			if i := f.liveHandle(key); i >= 0 {
				found[h] = f.cards[i].ID
			}
		}
	}
	return found, nil
}

func (f *Identity) UsersByPhoneHashes(_ context.Context, hashes [][]byte) (map[string]ids.UserID, error) {
	if err := f.begin("UsersByPhoneHashes", len(hashes), 0, identity.MaxPhoneHashes); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := make(map[string]ids.UserID, len(hashes))
	for _, h := range hashes {
		key := hex.EncodeToString(h)
		if id, ok := f.phones[key]; ok && f.matches(id, true) {
			found[key] = id
		}
	}
	return found, nil
}

func (f *Identity) UsersByXUserIDs(_ context.Context, xUserIDs []string) (map[string]ids.UserID, error) {
	if err := f.begin("UsersByXUserIDs", len(xUserIDs), 0, identity.MaxXUserIDs); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := make(map[string]ids.UserID, len(xUserIDs))
	for _, x := range xUserIDs {
		if id, ok := f.xUsers[x]; ok && f.matches(id, false) {
			found[x] = id
		}
	}
	return found, nil
}

func (f *Identity) MemberWallet(_ context.Context, id ids.UserID) (identity.MemberWallet, error) {
	const op = "MemberWallet"
	if err := f.Check(op); err != nil {
		return identity.MemberWallet{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.wallets, func(w identity.MemberWallet) bool { return w.UserID == id })
	if i < 0 || !f.walletVisible(f.wallets[i]) {
		return identity.MemberWallet{}, errs.New(errs.CodeUserNotFound, "fakes.Identity."+op)
	}
	return f.wallets[i], nil
}

func (f *Identity) MemberWallets(_ context.Context, after ids.UserID, limit int) ([]identity.MemberWallet, error) {
	if err := f.begin("MemberWallets", limit, 1, identity.MaxWalletPage); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	page := make([]identity.MemberWallet, 0, limit)
	for _, w := range f.wallets {
		if len(page) == limit {
			break
		}
		if compareUserIDs(w.UserID, after) > 0 && f.walletVisible(w) {
			page = append(page, w)
		}
	}
	return page, nil
}

func (f *Identity) begin(op string, n, lo, hi int) error {
	if n < lo || n > hi {
		return errs.New(errs.CodeInvalidInput, "fakes.Identity."+op,
			slog.Int("size", n), slog.Int("min", lo), slog.Int("max", hi))
	}
	return f.Check(op)
}

func (f *Identity) card(id ids.UserID) (identity.UserCard, bool) {
	if i := slices.IndexFunc(f.cards, func(c identity.UserCard) bool { return c.ID == id }); i >= 0 {
		return f.cards[i], true
	}
	return identity.UserCard{}, false
}

func (f *Identity) liveHandle(key string) int {
	return slices.IndexFunc(f.cards, func(c identity.UserCard) bool { return c.Handle == key && !c.Deleted })
}

func (f *Identity) walletVisible(w identity.MemberWallet) bool {
	card, known := f.card(w.UserID)
	return !known || !card.Deleted
}

func (f *Identity) matches(id ids.UserID, needsVerifiedPhone bool) bool {
	card, known := f.card(id)
	return known && card.AccountStatus == identity.AccountActive && !card.Deleted &&
		(card.PhoneVerified || !needsVerifiedPhone)
}

func identityHandleKey(raw string) (string, bool) {
	h, err := domain.ParseHandle(raw)
	return h.String(), err == nil
}

func compareUserIDs(a, b ids.UserID) int {
	x, y := a.UUID(), b.UUID()
	return bytes.Compare(x[:], y[:])
}
