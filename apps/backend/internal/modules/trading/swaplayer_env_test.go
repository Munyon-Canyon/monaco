package trading_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/jupiterfake"
)

const treasuryWallet = "wallet-treasury"

func usdcToken() platform.Mint { return platform.Mint{Address: usdcMint, Decimals: 6} }

func aaplxToken() platform.Mint { return platform.Mint{Address: aaplxMint, Decimals: 8} }

func jupiterMint(m platform.Mint) jupiter.Mint {
	return jupiter.Mint{Address: string(m.Address), Decimals: m.Decimals}
}

type hintLog struct {
	mu   sync.Mutex
	keys []string
	body [][]byte
}

func (h *hintLog) PublishHint(_ context.Context, key string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
	h.body = append(h.body, payload)
}

func (h *hintLog) published() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.keys...)
}

type signerFunc func(ctx context.Context, walletID string, unsigned []byte) ([]byte, platform.Signature, error)

func (f signerFunc) Sign(ctx context.Context, walletID string, unsigned []byte) ([]byte, platform.Signature, error) {
	return f(ctx, walletID, unsigned)
}

type layerEnv struct {
	pool   *pgxpool.Pool
	clk    *testkit.Clock
	ids    *testkit.IDs
	jup    *jupiterfake.Venue
	privy  *chainfake.Signer
	hints  *hintLog
	signer app.Signer
	venue  app.Venue
	reads  sqlc.DBTX
	uow    *db.UnitOfWork
}

func newLayerEnv(t *testing.T) *layerEnv {
	t.Helper()
	pool := testkit.DB(t)
	e := &layerEnv{
		pool: pool, clk: testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)), ids: testkit.NewIDs(7),
		jup: &jupiterfake.Venue{}, privy: &chainfake.Signer{}, hints: &hintLog{}, reads: pool,
	}
	e.uow = db.New(pool, e.ids, e.clk)
	e.signer = chain.NewSigner(e.privy)
	e.venue = chain.NewVenue(e.jup)
	e.script("req-1")
	return e
}

func (e *layerEnv) script(requestID string) {
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), jupiter.Order{
		RequestID: requestID, Transaction: chainfake.Unsigned(chainfake.WalletAddress(treasuryWallet)),
	})
}

func (e *layerEnv) layer() *app.SwapLayer {
	return app.NewSwapLayer(app.SwapLayerDeps{
		UoW: e.uow, Reads: e.reads, Clock: e.clk, IDs: e.ids, Venue: e.venue, Signer: e.signer, Hints: e.hints,
	})
}

func (e *layerEnv) request(source uuid.UUID) app.SwapRequest {
	return app.SwapRequest{
		Source:  domain.Source{Kind: domain.SourceProposal, ID: source},
		CabalID: ids.CabalIDFrom(e.ids.NewV7()),
		TreasuryWallet: app.TreasuryWallet{
			PrivyWalletID: treasuryWallet, Address: chainfake.WalletAddress(treasuryWallet),
		},
		Action: domain.ActionBuy, Symbol: "AAPLx", InMint: usdcToken(), OutMint: aaplxToken(),
		InAmount: 25_000_000, QuoteOutAmount: 105_000_000, SlippageBps: 100, SourceBatchSize: 1,
	}
}

func (e *layerEnv) run(t *testing.T, req app.SwapRequest) (app.SwapView, error) {
	t.Helper()
	return e.layer().Run(actorContext(t.Context()), req, nil)
}

func actorContext(ctx context.Context) context.Context {
	return observability.WithActor(ctx, "system:trade-engine")
}

func (e *layerEnv) swapIDs(t *testing.T, source uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT id FROM swaps WHERE source_id = $1 ORDER BY created_at, id`, source)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *layerEnv) events(t *testing.T, swap uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT type FROM events WHERE aggregate_id = $1 ORDER BY id`, swap)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *layerEnv) payload(t *testing.T, swap uuid.UUID, typ string) map[string]any {
	t.Helper()
	var raw json.RawMessage
	if err := e.pool.QueryRow(t.Context(),
		`SELECT payload FROM events WHERE aggregate_id = $1 AND type = $2`, swap, typ).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *layerEnv) assertAtMostOneTerminalEvent(t *testing.T) {
	t.Helper()
	var worst int
	if err := e.pool.QueryRow(t.Context(), `SELECT coalesce(max(n), 0) FROM (
		SELECT count(*) AS n FROM events WHERE type IN ('trade.confirmed', 'trade.failed') GROUP BY aggregate_id) c`).
		Scan(&worst); err != nil {
		t.Fatal(err)
	}
	if worst > 1 {
		t.Fatalf("a swap has %d terminal events, want at most 1", worst)
	}
}

func (e *layerEnv) row(t *testing.T, id uuid.UUID) (status string, signed []byte, requestID, signature *string) {
	t.Helper()
	if err := e.pool.QueryRow(t.Context(),
		`SELECT status, signed_tx, execute_request_id, tx_signature FROM swaps WHERE id = $1`, id).
		Scan(&status, &signed, &requestID, &signature); err != nil {
		t.Fatal(err)
	}
	return status, signed, requestID, signature
}

func (e *layerEnv) chainSigner() chain.Signer { return chain.NewSigner(e.privy) }

func (e *layerEnv) source() uuid.UUID { return e.ids.NewV7() }

func (e *layerEnv) beforeSigning(hook func(ctx context.Context) error) {
	signer := e.chainSigner()
	e.signer = signerFunc(func(ctx context.Context, w string, tx []byte) ([]byte, platform.Signature, error) {
		if err := hook(ctx); err != nil {
			return nil, "", err
		}
		return signer.Sign(ctx, w, tx)
	})
}
