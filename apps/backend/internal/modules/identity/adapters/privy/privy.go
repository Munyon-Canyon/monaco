package privy

import (
	"context"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

const maxE164Digits = 15

var (
	_ app.PrivyUsers    = Users{}
	_ app.PrivyDevUsers = Users{}
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

func (u Users) Create(ctx context.Context, email string) (app.PrivyUserID, error) {
	id, err := u.Client.CreateUser(ctx, email)
	if err != nil {
		return "", err
	}
	return app.PrivyUserID(id), nil
}

func (u Users) ByEmail(ctx context.Context, email string) (app.PrivyUserID, bool, error) {
	got, err := u.Client.UserByEmail(ctx, email)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return app.PrivyUserID(got.ID), true, nil
}

func (u Users) DevOnly(ctx context.Context, id app.PrivyUserID) (bool, bool, error) {
	got, err := u.Client.GetUser(ctx, chainprivy.UserID(id))
	if errs.CodeOf(err) == errs.CodeNotFound {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	_, dev := domain.DevSuffix(got.Email)
	return dev && got.Identities == 1, true, nil
}

func (u Users) Delete(ctx context.Context, id app.PrivyUserID) error {
	err := u.Client.DeleteUser(ctx, chainprivy.UserID(id))
	if errs.CodeOf(err) == errs.CodeNotFound {
		return nil
	}
	return err
}

func (u Users) Wallets(ctx context.Context, id app.PrivyUserID) ([]chain.SolanaAddress, error) {
	return u.Client.ListUserWallets(ctx, chainprivy.UserID(id))
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
		if got.X.UserID == "" {
			return app.PrivyUser{}, errs.New(errs.CodeDecodeFailed, "identity.PrivyUsers.User",
				slog.String("privy_user_id", string(id)))
		}
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
