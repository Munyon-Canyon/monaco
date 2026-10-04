package app_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type memberWallets struct{ identityport.WalletReader }

func (memberWallets) MemberWallet(_ context.Context, user ids.UserID) (identityport.MemberWallet, error) {
	return identityport.MemberWallet{UserID: user, PrivyWalletID: "privy-wallet", Address: "member"}, nil
}

func TestWalletReaderSigningWallet(t *testing.T) {
	t.Parallel()
	got, err := app.WalletReader{Reader: memberWallets{}}.SigningWallet(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7()))
	if err != nil || got != (chain.Wallet{ID: "privy-wallet", Address: "member"}) {
		t.Fatalf("SigningWallet = %+v, %v", got, err)
	}
}
