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
	"go.opentelemetry.io/otel/metric/noop"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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

type chainAccount struct {
	address chain.SolanaAddress
	exists  bool
	amount  uint64
	history []solana.SignatureInfo
}

type chainTx struct {
	kind  watchKind
	total uint64
	slot  uint64
}

type propertyChain struct {
	prefix    string
	wallet    chain.SolanaAddress
	accounts  []*chainAccount
	txs       map[chain.Signature]chainTx
	transfers map[chain.Signature][]solana.Transfer
	failures  map[chain.Signature]int
	slot      uint64
}

func (c *propertyChain) account(address chain.SolanaAddress) *chainAccount {
	for _, a := range c.accounts {
		if a.address == address {
			return a
		}
	}
	return nil
}

func (c *propertyChain) open() []*chainAccount {
	var open []*chainAccount
	for _, a := range c.accounts {
		if a.exists {
			open = append(open, a)
		}
	}
	return open
}

func (c *propertyChain) tick() uint64 {
	c.slot++
	return c.slot
}

func (c *propertyChain) record(kind watchKind, targets []*chainAccount, parts []uint64) {
	slot := c.tick()
	sig := chain.Signature(fmt.Sprintf("%s-%d", c.prefix, slot))
	info := solana.SignatureInfo{Signature: sig, Slot: slot, BlockTime: depositBlockTime(), Failed: kind == watchFailed}
	tx := chainTx{kind: kind, slot: slot}
	usdc := chain.Mint{Address: testkit.USDCMint, Decimals: 6}
	for i, target := range targets {
		target.history = slices.Insert(target.history, 0, info)
		part := parts[i]
		switch kind {
		case watchExternal, watchMonaco, watchFailed:
			c.transfers[sig] = append(c.transfers[sig], solana.Transfer{Mint: usdc, Net: money.NewBaseUnits(part, 6)})
			tx.total += part
			if kind != watchFailed {
				target.amount += part
			}
		case watchOtherMint:
			c.transfers[sig] = append(c.transfers[sig], solana.Transfer{
				Mint: chain.Mint{Address: "other-mint", Decimals: 6}, Net: money.NewBaseUnits(part, 6),
			})
		case watchOutflow:
			target.amount -= min(target.amount, part)
		}
	}
	c.txs[sig] = tx
}

func (c *propertyChain) states(addresses []chain.SolanaAddress) []solana.TokenAccountState {
	states := make([]solana.TokenAccountState, len(addresses))
	for i, address := range addresses {
		states[i] = solana.TokenAccountState{Address: address}
		if a := c.account(address); a != nil && a.exists {
			states[i] = solana.TokenAccountState{
				Address: address, Exists: true, Program: chain.SPLProgram, Mint: testkit.USDCMint,
				Owner: c.wallet, State: "initialized", Amount: money.NewBaseUnits(a.amount, 6),
			}
		}
	}
	return states
}

func (c *propertyChain) SignaturesFor(
	_ context.Context, address chain.SolanaAddress, opts solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	a := c.account(address)
	if a == nil {
		return nil, nil
	}
	return signaturesFromHistory(a.history, opts.Before, opts.Until, opts.Limit), nil
}

func (c *propertyChain) Accounts(
	_ context.Context, addresses []chain.SolanaAddress, _ uint64,
) (uint64, []solana.TokenAccountState, error) {
	return c.tick(), c.states(addresses), nil
}

func (c *propertyChain) TokenAccounts(
	context.Context, chain.SolanaAddress, chain.Mint,
) (uint64, []solana.TokenAccountState, error) {
	open := c.open()
	addresses := make([]chain.SolanaAddress, 0, len(open))
	for _, a := range open {
		addresses = append(addresses, a.address)
	}
	return c.tick(), c.states(addresses), nil
}

func (c *propertyChain) InboundTransfersForMint(
	_ context.Context, sig chain.Signature, _, _ chain.SolanaAddress,
) ([]solana.Transfer, error) {
	if c.failures[sig] > 0 {
		c.failures[sig]--
		return nil, errs.New(errs.CodeRPCUnavailable, "propertyChain.InboundTransfersForMint")
	}
	return c.transfers[sig], nil
}

func (c *propertyChain) OwnsSignature(_ context.Context, sig chain.Signature) (bool, error) {
	return c.txs[sig].kind == watchMonaco, nil
}

func TestDepositWatchJ1_CreditsExactlyTheExternalInboundTransfersAfterFirstSight(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var runs atomic.Uint64
	rapid.Check(t, func(rt *rapid.T) {
		run := runs.Add(1)
		user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
		now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
		prefix := fmt.Sprintf("w%d", run)
		canonical := canonicalAccount(t, user.Address)
		c := &propertyChain{
			prefix: prefix, wallet: user.Address, txs: map[chain.Signature]chainTx{},
			transfers: map[chain.Signature][]solana.Transfer{}, failures: map[chain.Signature]int{},
			accounts: []*chainAccount{{address: canonical}},
		}
		w := &watchRun{
			c: c, pool: pool, user: user, rt: rt, clock: testkit.NewClock(now),
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
	clock    *testkit.Clock
	rt       *rapid.T
	uow      *db.UnitOfWork
	watch    *app.DepositWatch
	resolver app.DepositCandidateResolver
	seen     []events.DepositCandidateSeen
}

func (w *watchRun) ctx() context.Context {
	return observability.WithActor(w.rt.Context(), "system:poller.funding.deposit_watch")
}

func (w *watchRun) start(run uint64) {
	w.watch = app.NewDepositWatch(
		w.pool, w.uow, testkit.NewIDs(20_000+run*10), w.clock,
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: w.user.ID, Address: w.user.Address}}),
		w.c, testkit.USDCMint, app.DepositPollInterval, unlimited(), 480,
		app.DepositWatchTuning{
			Rotation: 6 * time.Hour, RecoverySlots: 1 << 40, Discovery: 6 * time.Hour,
			Spread: func(period time.Duration) time.Duration { return period / 2 },
		},
		noop.Int64Counter{}, &stubLedger{},
	)
	w.resolver = app.NewDepositCandidateResolver(
		w.c, testkit.USDCMint, []fundingport.SignatureOwner{w.c},
		app.NewCreditDepositHandler(w.uow, &hints{}), testkit.NewIDs(30_000+run*10),
	)
	for range rapid.IntRange(0, 5).Draw(w.rt, "history") {
		w.mutate()
	}
	w.tick()
}

func (w *watchRun) mutate() {
	w.rt.Helper()
	switch rapid.IntRange(0, 5).Draw(w.rt, "mutation") {
	case 0:
		w.openAccount()
	case 1:
		w.closeAccount()
	default:
		w.appendTx()
	}
}

func (w *watchRun) openAccount() {
	var closed []*chainAccount
	for _, a := range w.c.accounts {
		if !a.exists {
			closed = append(closed, a)
		}
	}
	if len(w.c.accounts) < 3 {
		address := chain.SolanaAddress(fmt.Sprintf("%s-account-%d", w.c.prefix, len(w.c.accounts)))
		closed = append(closed, &chainAccount{address: address})
		w.c.accounts = append(w.c.accounts, closed[len(closed)-1])
	}
	if len(closed) > 0 {
		rapid.SampledFrom(closed).Draw(w.rt, "open").exists = true
	}
}

func (w *watchRun) watching(address chain.SolanaAddress) bool {
	var n int
	if err := w.pool.QueryRow(w.ctx(),
		`SELECT count(*) FROM deposit_watch_accounts WHERE token_account = $1`, string(address)).Scan(&n); err != nil {
		w.rt.Fatal(err)
	}
	return n == 1
}

func (w *watchRun) closeAccount() {
	var closable []*chainAccount
	for _, a := range w.c.open() {
		if w.watching(a.address) {
			closable = append(closable, a)
		}
	}
	if len(closable) == 0 {
		return
	}
	target := rapid.SampledFrom(closable).Draw(w.rt, "close")
	if target.amount > 0 {
		w.c.record(watchOutflow, []*chainAccount{target}, []uint64{target.amount})
	}
	target.exists = false
}

func (w *watchRun) appendTx() {
	open := w.c.open()
	if len(open) == 0 {
		return
	}
	kind := rapid.SampledFrom([]watchKind{
		watchExternal, watchExternal, watchExternal, watchOutflow, watchMonaco, watchOtherMint, watchFailed,
	}).Draw(w.rt, "kind")
	targets := rapid.SliceOfNDistinct(rapid.SampledFrom(open), 1, min(2, len(open)),
		func(a *chainAccount) chain.SolanaAddress { return a.address }).Draw(w.rt, "targets")
	parts := rapid.SliceOfN(rapid.Uint64Range(1, 5_000_000), len(targets), len(targets)).Draw(w.rt, "parts")
	w.c.record(kind, targets, parts)
}

func (w *watchRun) tick() {
	w.rt.Helper()
	if _, err := w.watch.Tick(w.ctx()); err != nil {
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

func (w *watchRun) deliver(e events.DepositCandidateSeen, mayFail bool) {
	w.rt.Helper()
	if mayFail && rapid.Bool().Draw(w.rt, "fetch fails") {
		w.c.failures[e.TxSignature] = 1
	}
	before := w.status(e)
	resolution, err := w.resolver.Fetch(w.ctx(), e)
	if err != nil {
		if after := w.status(e); before == "pending" && after != "pending" {
			w.rt.Fatalf("candidate %s moved from pending to %q after a fetch error", e.TxSignature, after)
		}
		return
	}
	err = w.uow.Do(w.ctx(), func(ctx context.Context, tx db.Tx) error {
		return w.resolver.Apply(ctx, tx, e, resolution, w.clock.Now())
	})
	if err != nil {
		w.rt.Fatalf("Apply %s = %v", e.TxSignature, err)
	}
}

func (w *watchRun) status(e events.DepositCandidateSeen) string {
	w.rt.Helper()
	var status string
	if err := w.pool.QueryRow(w.ctx(),
		`SELECT status FROM deposit_candidates WHERE tx_signature = $1 AND wallet_address = $2`,
		string(e.TxSignature), string(w.user.Address)).Scan(&status); err != nil {
		w.rt.Fatal(err)
	}
	return status
}

func (w *watchRun) drive() {
	for range rapid.IntRange(1, 14).Draw(w.rt, "steps") {
		switch rapid.IntRange(0, 3).Draw(w.rt, "action") {
		case 0:
			w.mutate()
		case 1:
			w.tick()
		case 2:
			w.clock.Advance(time.Duration(rapid.IntRange(1, 4).Draw(w.rt, "hours")) * time.Hour)
			w.tick()
		default:
			w.loadSeen()
			if len(w.seen) > 0 {
				w.deliver(rapid.SampledFrom(w.seen).Draw(w.rt, "delivery"), true)
			}
		}
	}
	for range 3 {
		w.clock.Advance(7 * time.Hour)
		w.tick()
		w.tick()
	}
	w.loadSeen()
	for _, e := range w.seen {
		w.deliver(e, false)
	}
	for _, e := range w.seen {
		if rapid.Bool().Draw(w.rt, "redeliver") {
			w.deliver(e, false)
		}
	}
}

func (w *watchRun) firstSeenSlot() uint64 {
	w.rt.Helper()
	var slot uint64
	if err := w.pool.QueryRow(w.ctx(),
		`SELECT first_seen_slot FROM deposit_watch_wallets WHERE wallet_address = $1`,
		string(w.user.Address)).Scan(&slot); err != nil {
		w.rt.Fatal(err)
	}
	return slot
}

func (w *watchRun) credited() map[chain.Signature]string {
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
	return got
}

func (w *watchRun) assertCredits() {
	w.rt.Helper()
	got := w.credited()
	first := w.firstSeenSlot()
	want := map[chain.Signature]string{}
	for sig, tx := range w.c.txs {
		if tx.kind == watchExternal && tx.slot >= first {
			want[sig] = strconv.FormatUint(tx.total, 10)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		w.rt.Fatalf("credited = %v, want %v", got, want)
	}
	var pending int
	if err := w.pool.QueryRow(w.ctx(),
		`SELECT count(*) FROM deposit_candidates WHERE wallet_address = $1 AND status = 'pending'`,
		string(w.user.Address)).Scan(&pending); err != nil || pending != 0 {
		w.rt.Fatalf("pending candidates = %d, %v; want 0", pending, err)
	}
}
