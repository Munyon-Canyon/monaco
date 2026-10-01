package privy

import (
	"context"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

const maxE164Digits = 15

var (
	_ app.PrivyUsers    = Users{}
	_ app.MemberWallets = Wallets{}
)

type Users struct {
	Client *chainprivy.Client
}

func (u Users) Verify(ctx context.Context, raw string) (app.PrivyUserID, error) {
	id, err := u.Client.VerifyAccessToken(ctx, raw)
	if err != nil {
		return "", err
	}
	return app.PrivyUserID(id), nil
}

func (u Users) User(ctx context.Context, id app.PrivyUserID) (app.PrivyUser, error) {
	got, err := u.Client.GetUser(ctx, chainprivy.UserID(id))
	if err != nil {
		return app.PrivyUser{}, err
	}
	phone, err := e164(got.Phone)
	if err != nil {
		return app.PrivyUser{}, err
	}
	user := app.PrivyUser{
		ID: app.PrivyUserID(got.ID), Email: got.Email, AppleEmail: got.AppleEmail, GoogleEmail: got.GoogleEmail,
		PhoneE164: phone,
	}
	if got.X != nil {
		user.X = &domain.XAccount{UserID: got.X.UserID, Username: got.X.Username}
	}
	return user, nil
}

func e164(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	number := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" -().", r) {
			return -1
		}
		return r
	}, raw)
	digits, plus := strings.CutPrefix(number, "+")
	if !plus || len(digits) < 2 || len(digits) > maxE164Digits || digits[0] == '0' ||
		strings.Trim(digits, "0123456789") != "" {
		return "", errs.New(errs.CodeDecodeFailed, "identity.PrivyUsers.User", slog.String("reason", "phone_not_e164"))
	}
	return number, nil
}

type Wallets struct {
	Client *chainprivy.Client
}

func (w Wallets) FindOrCreate(ctx context.Context, id app.PrivyUserID) (app.PrivyWallet, error) {
	got, err := w.Client.FindOrCreateMemberWallet(ctx, chainprivy.UserID(id))
	if err != nil {
		return app.PrivyWallet{}, err
	}
	return app.PrivyWallet{
		Wallet:       domain.Wallet{PrivyWalletID: got.ID, Address: got.Address},
		HasAppSigner: got.HasAppSigner,
	}, nil
}
