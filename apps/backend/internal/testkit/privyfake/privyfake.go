package privyfake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var (
	_ app.PrivyUsers    = (*Users)(nil)
	_ app.MemberWallets = (*Wallets)(nil)
)

type Users struct {
	testkit.Faults
	mu    sync.Mutex
	users map[app.PrivyUserID]app.PrivyUser
}

func (u *Users) Seed(user app.PrivyUser) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.users == nil {
		u.users = map[app.PrivyUserID]app.PrivyUser{}
	}
	u.users[user.ID] = user
}

func (u *Users) Verify(_ context.Context, raw string) (app.PrivyUserID, error) {
	if err := u.Check("Verify"); err != nil {
		return "", err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, ok := u.users[app.PrivyUserID(raw)]; !ok {
		return "", errs.New(errs.CodeUnauthorized, "privyfake.Users.Verify", slog.String("reason", "unknown_user"))
	}
	return app.PrivyUserID(raw), nil
}

func (u *Users) User(_ context.Context, id app.PrivyUserID) (app.PrivyUser, error) {
	if err := u.Check("User"); err != nil {
		return app.PrivyUser{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	user, ok := u.users[id]
	if !ok {
		return app.PrivyUser{}, errs.New(errs.CodeNotFound, "privyfake.Users.User")
	}
	return user, nil
}

type Wallets struct {
	testkit.Faults
	mu      sync.Mutex
	wallets map[app.PrivyUserID]app.PrivyWallet
	creates int
}

func (w *Wallets) Seed(id app.PrivyUserID, wallet app.PrivyWallet) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seed(id, wallet)
}

func (w *Wallets) seed(id app.PrivyUserID, wallet app.PrivyWallet) {
	if w.wallets == nil {
		w.wallets = map[app.PrivyUserID]app.PrivyWallet{}
	}
	w.wallets[id] = wallet
}

func (w *Wallets) FindOrCreate(_ context.Context, id app.PrivyUserID) (app.PrivyWallet, error) {
	if err := w.Check("FindOrCreate"); err != nil {
		return app.PrivyWallet{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if got, ok := w.wallets[id]; ok {
		return got, nil
	}
	sum := sha256.Sum256([]byte("member:" + string(id)))
	wallet := app.PrivyWallet{
		Wallet: domain.Wallet{
			PrivyWalletID: "wallet-" + hex.EncodeToString(sum[:6]),
			Address:       chain.AddressOf(sum[:]),
		},
		HasAppSigner: true,
	}
	w.seed(id, wallet)
	w.creates++
	return wallet, nil
}

func (w *Wallets) Creates() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.creates
}
