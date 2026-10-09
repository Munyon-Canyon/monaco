package funding_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type watchKind int

const (
	watchExternal watchKind = iota
	watchOutflow
	watchMonaco
	watchOtherMint
	watchFailed
)

type propertyChain struct {
	prefix    string
	history   []solana.SignatureInfo
	transfers map[chain.Signature][]solana.Transfer
	monaco    map[chain.Signature]bool
	next      uint64
}

func (c *propertyChain) append(kind watchKind, parts []uint64) chain.Signature {
	c.next++
	sig := chain.Signature(fmt.Sprintf("%s-%d", c.prefix, c.next))
	info := solana.SignatureInfo{
		Signature: sig, Slot: c.next, BlockTime: depositBlockTime(), Failed: kind == watchFailed,
	}
	c.history = slices.Insert(c.history, 0, info)
	usdc := chain.Mint{Address: testkit.USDCMint, Decimals: 6}
	switch kind {
	case watchExternal, watchMonaco, watchFailed:
		for _, part := range parts {
			c.transfers[sig] = append(c.transfers[sig], solana.Transfer{Mint: usdc, Net: money.NewBaseUnits(part, 6)})
		}
		c.monaco[sig] = kind == watchMonaco
	case watchOtherMint:
		c.transfers[sig] = []solana.Transfer{{
			Mint: chain.Mint{Address: "other-mint", Decimals: 6}, Net: money.NewBaseUnits(parts[0], 6),
		}}
	case watchOutflow:
	}
	return sig
}

func (c *propertyChain) InboundTransfersForMint(
	_ context.Context, sig chain.Signature, _, _ chain.SolanaAddress,
) ([]solana.Transfer, error) {
	return c.transfers[sig], nil
}

func (c *propertyChain) OwnsSignature(_ context.Context, sig chain.Signature) (bool, error) {
	return c.monaco[sig], nil
}

func TestDepositWatchJ1_CreditsExactlyTheExternalInboundTransfersAfterFirstSight(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var runs atomic.Uint64
	rapid.Check(t, func(rt *rapid.T) {
		run := runs.Add(1)
		user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
		now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
		c := &propertyChain{
			prefix: fmt.Sprintf("w%d", run), transfers: map[chain.Signature][]solana.Transfer{},
			monaco: map[chain.Signature]bool{},
		}
		w := &watchRun{
			c: c, pool: pool, user: user, now: now, rt: rt,
			uow: db.New(pool, testkit.NewIDs(10_000+run*10), testkit.NewClock(now)),
		}
		w.start(run)
		w.drive()
		w.assertCredits()
	})
}

type watchRun struct {
	c        *propertyChain
	pool     *pgxpool.Pool
	user     testkit.SeededUser
	now      time.Time
	rt       *rapid.T
	uow      *db.UnitOfWork
	poller   *app.DepositPoller
	resolver app.DepositCandidateResolver
	expected map[chain.Signature]uint64
	seen     []events.DepositCandidateSeen
}

func (w *watchRun) ctx() context.Context {
	return observability.WithActor(w.rt.Context(), "system:poller.funding.deposit_watch")
}

func (w *watchRun) start(run uint64) {
	rpc := &depositRPC{signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
		return signaturesFromHistory(w.c.history, before, until, limit)
	}}
	w.poller = app.NewDepositPoller(
		w.pool, w.uow, testkit.NewIDs(20_000+run*10), testkit.NewClock(w.now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: w.user.ID, Address: w.user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	w.resolver = app.NewDepositCandidateResolver(
		w.c, testkit.USDCMint, []fundingport.SignatureOwner{w.c},
		app.NewCreditDepositHandler(w.uow, &hints{}), testkit.NewIDs(30_000+run*10),
	)
	w.expected = map[chain.Signature]uint64{}
	for range rapid.IntRange(0, 4).Draw(w.rt, "history") {
		w.appendTx(false)
	}
	w.tick()
}

func (w *watchRun) appendTx(afterFirstSight bool) {
	kind := rapid.SampledFrom([]watchKind{
		watchExternal, watchExternal, watchOutflow, watchMonaco, watchOtherMint, watchFailed,
	}).Draw(w.rt, "kind")
	parts := rapid.SliceOfN(rapid.Uint64Range(1, 5_000_000), 1, 3).Draw(w.rt, "parts")
	if kind == watchOtherMint {
		parts = parts[:1]
	}
	sig := w.c.append(kind, parts)
	if kind != watchExternal || !afterFirstSight {
		return
	}
	var total uint64
	for _, part := range parts {
		total += part
	}
	w.expected[sig] = total
}

func (w *watchRun) tick() {
	w.rt.Helper()
	if _, err := w.poller.Tick(w.ctx()); err != nil {
		w.rt.Fatalf("Tick = %v", err)
	}
}

func (w *watchRun) loadSeen() {
	w.rt.Helper()
	rows, err := w.pool.Query(w.ctx(),
		`SELECT payload FROM events WHERE type = $1 AND payload->>'wallet_address' = $2 ORDER BY id`,
		events.TypeDepositCandidateSeen, string(w.user.Address))
	if err != nil {
		w.rt.Fatal(err)
	}
	defer rows.Close()
	w.seen = w.seen[:0]
	for rows.Next() {
		var payload []byte
		var e events.DepositCandidateSeen
		if err := rows.Scan(&payload); err != nil {
			w.rt.Fatal(err)
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			w.rt.Fatal(err)
		}
		w.seen = append(w.seen, e)
	}
	if err := rows.Err(); err != nil {
		w.rt.Fatal(err)
	}
}

func (w *watchRun) deliver(e events.DepositCandidateSeen) {
	w.rt.Helper()
	resolution, err := w.resolver.Fetch(w.ctx(), e)
	if err != nil {
		w.rt.Fatalf("Fetch %s = %v", e.TxSignature, err)
	}
	err = w.uow.Do(w.ctx(), func(ctx context.Context, tx db.Tx) error {
		return w.resolver.Apply(ctx, tx, e, resolution, w.now)
	})
	if err != nil {
		w.rt.Fatalf("Apply %s = %v", e.TxSignature, err)
	}
}

func (w *watchRun) drive() {
	for range rapid.IntRange(1, 14).Draw(w.rt, "steps") {
		switch rapid.IntRange(0, 2).Draw(w.rt, "action") {
		case 0:
			w.appendTx(true)
		case 1:
			w.tick()
		default:
			w.loadSeen()
			if len(w.seen) > 0 {
				w.deliver(rapid.SampledFrom(w.seen).Draw(w.rt, "delivery"))
			}
		}
	}
	w.tick()
	w.loadSeen()
	for _, e := range w.seen {
		w.deliver(e)
	}
	for _, e := range w.seen {
		if rapid.Bool().Draw(w.rt, "redeliver") {
			w.deliver(e)
		}
	}
}

func (w *watchRun) assertCredits() {
	w.rt.Helper()
	rows, err := w.pool.Query(w.ctx(),
		`SELECT tx_signature, amount_micros::text FROM deposits WHERE wallet_address = $1`, string(w.user.Address))
	if err != nil {
		w.rt.Fatal(err)
	}
	defer rows.Close()
	got := map[chain.Signature]string{}
	for rows.Next() {
		var sig chain.Signature
		var amount string
		if err := rows.Scan(&sig, &amount); err != nil {
			w.rt.Fatal(err)
		}
		if _, dup := got[sig]; dup {
			w.rt.Fatalf("%s credited twice", sig)
		}
		got[sig] = amount
	}
	if err := rows.Err(); err != nil {
		w.rt.Fatal(err)
	}
	want := map[chain.Signature]string{}
	for sig, amount := range w.expected {
		want[sig] = strconv.FormatUint(amount, 10)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		w.rt.Fatalf("credited = %v, want %v", got, want)
	}
	for sig := range got {
		if w.c.monaco[sig] {
			w.rt.Fatalf("Monaco-signed %s was credited", sig)
		}
	}
	var pending int
	if err := w.pool.QueryRow(w.ctx(),
		`SELECT count(*) FROM deposit_candidates WHERE wallet_address = $1 AND status = 'pending'`,
		string(w.user.Address)).Scan(&pending); err != nil || pending != 0 {
		w.rt.Fatalf("pending candidates = %d, %v; want 0", pending, err)
	}
}
