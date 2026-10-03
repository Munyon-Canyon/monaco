package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	usdcMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	usdc     = domain.Asset(usdcMint)
	aapl     = domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
)

type fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	ids    *testkit.IDs
	uow    *db.UnitOfWork
	clock  *testkit.Clock
	ledger app.Ledger
	logs   *testkit.Logs
	cfg    config.Config
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return newFixtureOn(t, testkit.Config())
}

func newFixtureOn(t *testing.T, cfg config.Config) fixture {
	t.Helper()
	g := testkit.NewIDs(7)
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	return fixture{
		t: t, pool: pool, ids: g, uow: db.New(pool, g, clk), clock: clk,
		ledger: app.NewLedger(chain.SolanaAddress(cfg.Solana.USDCMint), clk), logs: &testkit.Logs{}, cfg: cfg,
	}
}

func (f fixture) usdc() domain.Asset {
	return domain.MintAsset(chain.SolanaAddress(f.cfg.Solana.USDCMint))
}

func (f fixture) ctx() context.Context {
	return observability.WithLogger(f.t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, f.logs))
}

func (f fixture) cabal(t *testing.T) ids.CabalID {
	t.Helper()
	id, err := ids.ParseCabalID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) user(t *testing.T) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) do(fn func(ctx context.Context, tx db.Tx) error) error {
	return f.uow.Do(f.ctx(), fn)
}

func (f fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f fixture) drift(t *testing.T) []string {
	t.Helper()
	out, err := f.findDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (f fixture) findDrift(ctx context.Context) ([]string, error) {
	return treasury.LedgerCheck(f.cfg).Check(ctx, f.pool)
}

func amount(v int64) money.SignedMicros { return money.SignedMicrosFromInt64(v) }

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want code %q", code)
	}
	if got := errs.CodeOf(err); got != code {
		t.Fatalf("code = %q, want %q (err %v)", got, code, err)
	}
}

func (f fixture) fund(
	user ids.UserID, cabal ids.CabalID, micros, shares int64, status domain.TxnStatus,
) (domain.UserTxn, domain.CabalTxn, error) {
	id := f.ids.NewV7()
	s := domain.SharesAsset(cabal)
	u, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, CabalID: cabal, Kind: domain.UserFund, Status: status, TransferID: id,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: f.usdc(), Amount: amount(-micros)},
		{Account: domain.UserCabal, Asset: f.usdc(), Amount: amount(micros)},
		{Account: domain.UserHolder, Asset: s, Amount: amount(shares)},
		{Account: domain.UserIssuer, Asset: s, Amount: amount(-shares)},
	})
	if err != nil {
		return domain.UserTxn{}, domain.CabalTxn{}, err
	}
	c, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalFund, Status: status, TransferID: id,
	}, []domain.CabalEntry{
		{Account: domain.CabalMembers, Asset: f.usdc(), Amount: amount(-micros)},
		{Account: domain.CabalTreasury, Asset: f.usdc(), Amount: amount(micros)},
	})
	return u, c, err
}

func (f fixture) cashOut(
	user ids.UserID, cabal ids.CabalID, micros, shares int64, status domain.TxnStatus,
) (domain.UserTxn, domain.CabalTxn, error) {
	u, c, err := f.fund(user, cabal, -micros, -shares, status)
	if err != nil {
		return u, c, err
	}
	u.Kind, c.Kind = domain.UserCashOut, domain.CabalCashOut
	return u, c, nil
}

func (f fixture) swap(cabal ids.CabalID, micros, units int64) (domain.CabalTxn, error) {
	return domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: f.ids.NewV7(),
		TxSignature: "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: f.usdc(), Amount: amount(-micros)},
		{Account: domain.CabalVenue, Asset: f.usdc(), Amount: amount(micros)},
		{Account: domain.CabalTreasury, Asset: aapl, Amount: amount(units)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: amount(-units)},
	})
}

func (f fixture) postPair(u domain.UserTxn, c domain.CabalTxn) error {
	return f.do(func(ctx context.Context, tx db.Tx) error {
		if err := f.ledger.PostUserTxn(ctx, tx, u); err != nil {
			return err
		}
		return f.ledger.PostCabalTxn(ctx, tx, c)
	})
}

func (f fixture) postCabal(c domain.CabalTxn) error {
	return f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostCabalTxn(ctx, tx, c) })
}
