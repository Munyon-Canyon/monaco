package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type failing struct{}

var errWatchDown = errs.New(errs.CodeInternal, "test.down")

func (failing) OwnsSignature(context.Context, chain.Signature) (bool, error) {
	return false, errWatchDown
}

func (failing) InboundTransfers(context.Context, chain.Signature, chain.SolanaAddress) ([]solana.Transfer, error) {
	return nil, errWatchDown
}

func (failing) AssetByMint(context.Context, market.Mint) (market.Asset, error) {
	return market.Asset{}, errWatchDown
}

func (failing) LatestPrices(context.Context) (map[market.AssetID]market.Price, error) {
	return nil, errWatchDown
}

func (failing) MemberWallet(context.Context, ids.UserID) (identityport.MemberWallet, error) {
	return identityport.MemberWallet{}, errWatchDown
}

func (failing) MemberWallets(context.Context, ids.UserID, int) ([]identityport.MemberWallet, error) {
	return nil, errWatchDown
}

func TestDetectExternalDeposit_FailuresRecordNothing(t *testing.T) {
	t.Parallel()
	usdc := func(t *testing.T, e *watchEnv) chain.Signature {
		t.Helper()
		return e.inbound(t, e.usdcMint(), 5_000_000, randomAddress(t))
	}
	cases := map[string]func(t *testing.T, e *watchEnv) (context.Context, chain.Signature){
		"seen read fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			exec(t, e.pool, `ALTER TABLE external_deposits RENAME TO external_deposits_gone`)
			return actor(t), usdc(t, e)
		},
		"an owner fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			e.deps.Owners = []fundingport.SignatureOwner{failing{}}
			return actor(t), usdc(t, e)
		},
		"the chain read fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			e.deps.Chain = failing{}
			return actor(t), usdc(t, e)
		},
		"the chain returns a bad mint": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			return actor(t), e.inbound(t, chain.Mint{Address: "0OIl", Decimals: 6}, 1, randomAddress(t))
		},
		"the catalog fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			e.deps.Assets = failing{}
			return actor(t), e.inbound(t, e.stockMint(), 1, randomAddress(t))
		},
		"prices fail": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			e.deps.Prices = failing{}
			return actor(t), e.inbound(t, e.stockMint(), 1, randomAddress(t))
		},
		"the wallet lookup fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			e.deps.Wallets = failing{}
			return actor(t), usdc(t, e)
		},
		"an ignored row insert fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			refuseInserts(t, e)
			return actor(t), e.inbound(t, e.usdcMint(), 1, randomAddress(t))
		},
		"the pause write fails": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			exec(t, e.pool, `ALTER TABLE cabal_pauses RENAME TO cabal_pauses_gone`)
			return actor(t), usdc(t, e)
		},
		"the event append fails without an actor": func(t *testing.T, e *watchEnv) (context.Context, chain.Signature) {
			t.Helper()
			return t.Context(), usdc(t, e)
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newWatchEnv(t)
			ctx, sig := arrange(t, env)

			res, err := env.try(ctx, sig, domain.SourceWebhook)

			if err == nil || res.Recorded {
				t.Fatalf("Handle = %+v, %v, want an error and nothing recorded", res, err)
			}
			if countRows(t, env, "external_deposits") != 0 || countRows(t, env, "cabal_pauses") != 0 {
				t.Fatal("a failed detection left a row behind")
			}
		})
	}
}

func actor(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:webhook.privy")
}

func refuseInserts(t *testing.T, e *watchEnv) {
	t.Helper()
	exec(t, e.pool, `CREATE FUNCTION refuse_insert() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$`)
	exec(t, e.pool, `CREATE TRIGGER refuse_insert BEFORE INSERT ON external_deposits
		FOR EACH ROW EXECUTE FUNCTION refuse_insert()`)
}

func countRows(t *testing.T, e *watchEnv, table string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func TestDetectExternalDeposit_FindsTheSenderOnALaterWalletPage(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	full := make([]identityport.MemberWallet, identityport.MaxWalletPage)
	for i := range full {
		full[i] = identityport.MemberWallet{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), Address: randomAddress(t)}
	}
	sender := identityport.MemberWallet{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), Address: randomAddress(t)}
	env.deps.Wallets = &watchWallets{pages: [][]identityport.MemberWallet{full, {sender}}}

	env.handle(t, env.inbound(t, env.usdcMint(), 5_000_000, sender.Address), domain.SourceWebhook)

	if e := detectedEvent(t, env.pool); e.SenderUserID == nil || *e.SenderUserID != sender.UserID.UUID() {
		t.Fatalf("sender_user_id = %v, want %s from the second page", e.SenderUserID, sender.UserID)
	}
}
