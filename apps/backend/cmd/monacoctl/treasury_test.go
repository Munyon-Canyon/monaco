package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	treasuryapp "github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	refundTreasury = chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
	refundMember   = chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P")
)

type refundChain struct {
	sent     int
	buildErr error
}

func (c *refundChain) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	return money.Micros{}, errs.New(errs.CodePriceUnavailable, "test.PotValue")
}

func (c *refundChain) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	return []solana.Status{{Signature: sigs[0], State: solana.StateFinalized}}, nil
}

func (c *refundChain) Build(context.Context, relayer.TransferSpec) (relayer.SignedTx, error) {
	if c.buildErr != nil {
		return relayer.SignedTx{}, c.buildErr
	}
	return relayer.SignedTx{Bytes: []byte("tx"), Signature: "refund-sig", LastValidBlockHeight: 10}, nil
}

func (c *refundChain) Broadcast(context.Context, relayer.SignedTx) error {
	c.sent++
	return nil
}

func (c *refundChain) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(50_000_000, 6), nil
}

func (c *refundChain) PublishHint(context.Context, string, []byte) {}

func (c *refundChain) TreasuryWallet(context.Context, ids.CabalID) (chain.Wallet, error) {
	return chain.Wallet{ID: "treasury-wallet", Address: refundTreasury}, nil
}

func (c *refundChain) MemberAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return refundMember, nil
}

func fakeRefundPorts(c *refundChain) refundPorts {
	return func(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock) *treasuryapp.FundRefunds {
		return treasuryapp.NewFundRefunds(treasuryapp.FundRefundDeps{
			Reads:     pool,
			UoW:       uow,
			IDs:       ids.Real{},
			Clock:     clk,
			Ledger:    treasuryapp.NewLedger(chain.SolanaAddress(cfg.Solana.USDCMint), clk),
			Pot:       c,
			Chain:     c,
			Transfers: func() (treasuryapp.PayoutTransfers, error) { return c, nil },
			Wallets:   c,
			Balances:  c,
			USDC:      chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: 6},
			Hints:     c,
		})
	}
}

func refundTool(t *testing.T, input string) (tool, *pgxpool.Pool, *refundChain) {
	t.Helper()
	pool := testkit.DB(t)
	c := &refundChain{}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(), "USER=alice",
	}
	now := time.Now().UTC().Truncate(time.Second)
	return treasuryTool(environ, testkit.NewClock(now), strings.NewReader(input), fakeRefundPorts(c)), pool, c
}

func landedTransfer(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(t.Context(), `INSERT INTO fund_transfers (id, user_id, cabal_id, amount_micros, from_address,
		to_address, status, signed_tx, tx_signature, last_valid_block_height, created_at, submitted_at, landed_at)
		VALUES ($1, $2, $3, 7000000, 'm', 't', 'landed', '\x01', $4, 5, now(), now(), now())`,
		id, uuid.New(), uuid.New(), "fund-sig-"+id.String()); err != nil {
		t.Fatal(err)
	}
	return id
}

func runTreasury(t *testing.T, tl tool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := tl(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestTreasuryRefundFund_rejectsBadArguments(t *testing.T) {
	t.Parallel()
	tl, _, _ := refundTool(t, "")
	for _, args := range [][]string{
		{"refund-fund"},
		{"refund-fund", "--transfer", "nope", "--reason", "x"},
		{"refund-fund", "--transfer", uuid.NewString()},
		{"refund-fund", "--transfer", uuid.NewString(), "--reason", "x", "extra"},
	} {
		if code, _, stderr := runTreasury(t, tl, args...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Fatalf("%v = %d, %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestTreasuryRefundFund_dryRunPrintsThePlanAndMovesNothing(t *testing.T) {
	t.Parallel()
	tl, pool, c := refundTool(t, "")
	id := landedTransfer(t, pool)
	code, out, stderr := runTreasury(t, tl, "refund-fund", "--transfer", id.String(), "--reason", "stuck", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run = %d, %q", code, stderr)
	}
	wantContains(t, "stdout", out, "plan transfer="+id.String(), "amount_micros=7000000",
		"from="+string(refundTreasury), "to="+string(refundMember), "treasury_micros=50000000")
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM fund_transfers WHERE id = $1`, id).
		Scan(&status); err != nil ||
		status != "landed" ||
		c.sent != 0 {
		t.Fatalf("status %q, sent %d (%v), want landed and nothing sent", status, c.sent, err)
	}
}

func TestTreasuryRefundFund_cancelledWhenNotConfirmed(t *testing.T) {
	t.Parallel()
	tl, pool, c := refundTool(t, "n\n")
	id := landedTransfer(t, pool)
	code, _, stderr := runTreasury(t, tl, "refund-fund", "--transfer", id.String(), "--reason", "stuck")
	if code != 1 || !strings.Contains(stderr, "cancelled") || c.sent != 0 {
		t.Fatalf("declined = %d, %q, sent %d, want 1, cancelled and nothing sent", code, stderr, c.sent)
	}
}

func TestTreasuryRefundFund_refundsOnceAndTheSecondRunMovesNothing(t *testing.T) {
	t.Parallel()
	tl, pool, c := refundTool(t, "y\n")
	id := landedTransfer(t, pool)
	code, out, stderr := runTreasury(t, tl, "refund-fund", "--transfer", id.String(), "--reason", "stuck")
	if code != 0 {
		t.Fatalf("refund = %d, %q", code, stderr)
	}
	wantContains(t, "stdout", out, "signature=refund-sig", "state=refunded")
	code, out, stderr = runTreasury(t, tl, "refund-fund", "--transfer", id.String(), "--reason", "again", "--yes")
	if code != 0 {
		t.Fatalf("second refund = %d, %q", code, stderr)
	}
	wantContains(t, "second stdout", out, "state=already_refunded")
	var status, actor string
	var events int
	if err := pool.QueryRow(t.Context(), `SELECT f.status, e.payload->>'actor', (SELECT count(*) FROM events
		WHERE type = 'cabal.fund_refunded') FROM fund_transfers f JOIN events e ON e.aggregate_id = f.id
		WHERE f.id = $1 AND e.type = 'cabal.fund_refunded'`, id).Scan(&status, &actor, &events); err != nil ||
		status != "refunded" || actor != "operator:alice" || events != 1 || c.sent != 1 {
		t.Fatalf("status %q, actor %q, events %d, sent %d (%v), want one refund by operator:alice",
			status, actor, events, c.sent, err)
	}
}

func TestTreasuryRefundFund_failsOnATransferThatIsNotLanded(t *testing.T) {
	t.Parallel()
	tl, _, _ := refundTool(t, "")
	code, _, stderr := runTreasury(t, tl, "refund-fund", "--transfer", uuid.NewString(), "--reason", "stuck", "--yes")
	if code != 1 || !strings.Contains(stderr, "monacoctl:") {
		t.Fatalf("unknown transfer = %d, %q, want 1", code, stderr)
	}
}

func TestTreasuryTool_isWiredToTheLiveModules(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	cfg.Solana.RPCURL = "http://127.0.0.1/rpc/"
	cfg.Timeouts = config.Timeouts{Privy: time.Second, RPC: time.Second}
	pool := testkit.DB(t)
	clk := clock.Real{}
	if liveRefundPorts(cfg, pool, db.New(pool, ids.Real{}, clk), clk) == nil {
		t.Fatal("liveRefundPorts = nil")
	}
	if toolTreasury(toolEnv{}) == nil {
		t.Fatal("toolTreasury = nil")
	}
}

func TestTreasuryRefundFund_reportsAFailedRefundAndAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	tl, pool, c := refundTool(t, "")
	c.buildErr = errs.New(errs.CodePrivyUnavailable, "test.Build")
	id := landedTransfer(t, pool)
	code, _, stderr := runTreasury(t, tl, "refund-fund", "--transfer", id.String(), "--reason", "stuck", "--yes")
	if code != 1 || !strings.Contains(stderr, "monacoctl:") || c.sent != 0 {
		t.Fatalf("failed refund = %d, %q, sent %d, want 1 and nothing sent", code, stderr, c.sent)
	}
	broken := treasuryTool(
		[]string{
			"MONACO_ENV=test", "NATS_URL=" + testkit.NATSURL(),
			"DATABASE_URL=postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1",
		},
		clock.Real{}, strings.NewReader(""), fakeRefundPorts(c))
	code, _, stderr = runTreasury(t, broken, "refund-fund", "--transfer", id.String(), "--reason", "stuck")
	if code != 1 || !strings.Contains(stderr, "monacoctl:") {
		t.Fatalf("unreachable database = %d, %q, want 1", code, stderr)
	}
}
