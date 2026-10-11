package relayer_test

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type sent struct {
	to       chain.SolanaAddress
	lamports uint64
}

type solFake struct {
	testkit.Faults
	mu       sync.Mutex
	payer    chain.SolanaAddress
	balances map[chain.SolanaAddress]uint64
	sends    []sent
	statuses []solana.State
	failed   bool
	expire   bool
	unlanded bool
	hashes   uint64
	sigs     map[chain.Signature]bool
	beforeIn func()
	onSend   func()
}

func newSOLFake(payer chain.SolanaAddress, wallet, relayerLamports uint64) *solFake {
	return &solFake{payer: payer, sigs: map[chain.Signature]bool{}, balances: map[chain.SolanaAddress]uint64{
		payer: relayerLamports, memberWallet: wallet,
	}}
}

func (f *solFake) SOLBalance(_ context.Context, addr chain.SolanaAddress) (money.BaseUnits, error) {
	op := "SOLBalance:wallet"
	if addr == f.payer {
		op = "SOLBalance:relayer"
	}
	if err := f.Check(op); err != nil {
		return money.BaseUnits{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return money.NewBaseUnits(f.balances[addr], 9), nil
}

func (f *solFake) LatestBlockhash(context.Context) (solana.Blockhash, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hashes++
	hash := solana.Blockhash{LastValidBlockHeight: 100}
	binary.LittleEndian.PutUint64(hash.Hash[:], f.hashes)
	return hash, f.Check("LatestBlockhash")
}

func (f *solFake) SendTransaction(_ context.Context, raw []byte) (chain.Signature, error) {
	if err := f.Check("SendTransaction"); err != nil {
		return "", err
	}
	tx, err := chain.DecodeTransaction(raw)
	if err != nil || !tx.Signed(0) || tx.Signers[0] != f.payer {
		return "", errors.Join(err, errs.New(errs.CodeInvalidInput, "solFake.SendTransaction"))
	}
	lamports := binary.LittleEndian.Uint64(tx.Message[len(tx.Message)-8:])
	to := chain.SolanaAddress(chain.EncodeBase58(tx.Message[4+32 : 4+64]))
	f.mu.Lock()
	f.sends = append(f.sends, sent{to: to, lamports: lamports})
	f.sigs[chain.SignatureOf(tx.Signatures[0])] = !f.unlanded
	before := f.beforeIn
	f.mu.Unlock()
	if before != nil {
		before()
	}
	f.mu.Lock()
	if !f.unlanded {
		f.balances[to] += lamports
		f.balances[f.payer] -= lamports
	}
	f.mu.Unlock()
	if f.onSend != nil {
		f.onSend()
	}
	return chain.SignatureOf(tx.Signatures[0]), nil
}

func (f *solFake) SignatureStatuses(_ context.Context, sigs []chain.Signature) ([]solana.Status, error) {
	if err := f.Check("SignatureStatuses"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	state := solana.StateNotFound
	if f.sigs[sigs[0]] {
		state = solana.StateProcessing
	}
	if len(f.statuses) > 0 {
		state, f.statuses = f.statuses[0], f.statuses[1:]
	}
	height := uint64(50)
	if f.expire {
		height = 200
	}
	return []solana.Status{{Signature: sigs[0], State: state, Failed: f.failed, BlockHeight: height}}, nil
}

func (f *solFake) sentList() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.sends...)
}

type instant struct{ clock.Real }

func (instant) After(time.Duration) <-chan time.Time {
	c := make(chan time.Time, 1)
	c <- time.Time{}
	return c
}

type never struct{ clock.Real }

func (never) After(time.Duration) <-chan time.Time { return nil }

type topUpEnv struct {
	pool    *pgxpool.Pool
	rpc     *solFake
	uow     *db.UnitOfWork
	relayer *relayer.Relayer
	ids     *testkit.IDs
	topUps  *relayer.TopUps
}

func (e *topUpEnv) with(clk clock.Clock) *relayer.TopUps {
	return relayer.NewTopUps(e.relayer, e.rpc, e.uow, clk, e.ids)
}

func (e *topUpEnv) exec(t *testing.T, stmt string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), stmt, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *solFake) drain(wallet chain.SolanaAddress) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balances[wallet] = 0
}

type brokenAfter struct {
	instant
	mu sync.Mutex
	n  int
}

func (b *brokenAfter) Now() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n--
	if b.n <= 0 {
		return time.Date(-5000, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return b.instant.Now()
}

func newTopUpEnv(t *testing.T, wallet, relayerLamports uint64, clk clock.Clock) *topUpEnv {
	t.Helper()
	r, err := relayer.New(keyConfig("relayer"), nil)
	if err != nil {
		t.Fatal(err)
	}
	rpc := newSOLFake(r.Address(), wallet, relayerLamports)
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), clock.Real{})
	e := &topUpEnv{pool: pool, rpc: rpc, uow: uow, relayer: r, ids: testkit.NewIDs(3)}
	e.topUps = e.with(clk)
	return e
}

const funded = 1_000_000_000

func TestTopUpSOL_bringsAWalletBelowTheFloorToTheTargetOnce(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 1_500_000, funded, instant{})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	for range 2 {
		if err := e.topUps.TopUpSOL(ctx, memberWallet); err != nil {
			t.Fatal(err)
		}
	}

	got := e.rpc.sentList()
	if len(got) != 1 || got[0] != (sent{to: memberWallet, lamports: 2_500_000}) {
		t.Fatalf("sends = %+v, want one of 2_500_000 lamports to reach the 0.004 SOL target", got)
	}
	if n := len(logs.Lines(observability.RelayerSOLToppedUp.Name, "wallet")); n != 1 {
		t.Fatalf("%d top-up logs, want one", n)
	}
}

func TestTopUpSOL_sendsNothingAtOrAboveTheFloor(t *testing.T) {
	t.Parallel()
	for name, lamports := range map[string]uint64{"floor": relayer.TopUpFloorLamports, "target": relayer.TopUpTargetLamports} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newTopUpEnv(t, lamports, funded, instant{})
			if err := e.topUps.TopUpSOL(t.Context(), memberWallet); err != nil || len(e.rpc.sentList()) != 0 {
				t.Fatalf("err %v, sends %v, want nothing sent", err, e.rpc.sentList())
			}
		})
	}
}

func TestTopUpSOL_aSecondCallerDuringTheSendSendsNothing(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, instant{})
	var second error
	e.rpc.beforeIn = func() {
		e.rpc.beforeIn = nil
		second = e.topUps.TopUpSOL(t.Context(), memberWallet)
	}

	first := e.topUps.TopUpSOL(t.Context(), memberWallet)

	if first != nil || second != nil || len(e.rpc.sentList()) != 1 {
		t.Fatalf("errs %v, %v, sends %v, want both done on exactly one send", first, second, e.rpc.sentList())
	}
}

func TestTopUpSOL_aRelayerBelowItsFloorAlertsAndSendsNothing(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, relayer.FloorLamports+relayer.TopUpTargetLamports, instant{})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	err := e.topUps.TopUpSOL(ctx, memberWallet)

	if !errs.Retryable(errs.CodeOf(err)) || len(e.rpc.sentList()) != 0 {
		t.Fatalf("err %v, sends %v, want a retryable refusal and nothing sent", err, e.rpc.sentList())
	}
	if lines := logs.Lines(
		observability.RelayerTopUpUnderfunded.Name,
		"level",
	); len(lines) != 1 ||
		lines[0] != "ERROR" {
		t.Fatalf("underfunded alerts = %q, want one", lines)
	}
}

func TestTopUpSOL_waitsForTheTransferToLand(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, instant{})
	e.rpc.statuses = []solana.State{solana.StateNotFound, solana.StateNotFound, solana.StateProcessing}

	if err := e.topUps.TopUpSOL(t.Context(), memberWallet); err != nil || len(e.rpc.statuses) != 0 {
		t.Fatalf("err %v with %d statuses unread, want the wait to reach the landed transfer", err, len(e.rpc.statuses))
	}
}

func TestTopUpSOL_refusesWhatItCannotDoSafely(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeRPCUnavailable, "solana")
	for name, c := range map[string]struct {
		arrange func(e *topUpEnv)
		wallet  chain.SolanaAddress
		want    errs.Code
	}{
		"bad wallet":        {wallet: "not-an-address", want: errs.CodeInvalidAddress},
		"balance read":      {arrange: func(e *topUpEnv) { e.rpc.FailOnce("SOLBalance:wallet", down) }, want: errs.CodeRPCUnavailable},
		"recheck read":      {arrange: func(e *topUpEnv) { e.rpc.FailOnce("SOLBalance:wallet", nil); e.rpc.FailOnce("SOLBalance:wallet", down) }, want: errs.CodeRPCUnavailable},
		"relayer read":      {arrange: func(e *topUpEnv) { e.rpc.Fail("SOLBalance:relayer", down) }, want: errs.CodeRPCUnavailable},
		"blockhash":         {arrange: func(e *topUpEnv) { e.rpc.Fail("LatestBlockhash", down) }, want: errs.CodeRPCUnavailable},
		"send":              {arrange: func(e *topUpEnv) { e.rpc.Fail("SendTransaction", down) }, want: errs.CodeRPCUnavailable},
		"status":            {arrange: func(e *topUpEnv) { e.rpc.Fail("SignatureStatuses", down) }, want: errs.CodeRPCUnavailable},
		"landed read":       {arrange: func(e *topUpEnv) { e.rpc.onSend = func() { e.rpc.Fail("SOLBalance:wallet", down) } }, want: errs.CodeRPCUnavailable},
		"failed on chain":   {arrange: func(e *topUpEnv) { e.rpc.failed = true }, want: errs.CodeUpstreamUnavailable},
		"blockhash expired": {arrange: func(e *topUpEnv) { e.rpc.statuses = []solana.State{solana.StateNotFound}; e.rpc.expire = true }, want: errs.CodeUpstreamUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newTopUpEnv(t, 0, funded, instant{})
			if c.arrange != nil {
				c.arrange(e)
			}
			wallet := memberWallet
			if c.wallet != "" {
				wallet = c.wallet
			}
			if err := e.topUps.TopUpSOL(t.Context(), wallet); errs.CodeOf(err) != c.want {
				t.Fatalf("err = %v, want %s", err, c.want)
			}
		})
	}
}

func (e *topUpEnv) journal(t *testing.T) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT status FROM relayer_sol_topups ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestTopUpSOL_aCallerThatStopsWaitingLeavesTheTopUpInFlightForTheNext(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, never{})
	ctx, cancel := context.WithCancel(t.Context())
	e.rpc.unlanded, e.rpc.onSend = true, cancel

	if err := e.topUps.TopUpSOL(ctx, memberWallet); errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("first err = %v, want upstream_timeout", err)
	}
	next, stop := context.WithCancel(t.Context())
	e.rpc.onSend = stop
	err := e.topUps.TopUpSOL(next, memberWallet)
	if !errs.Retryable(errs.CodeOf(err)) || len(e.rpc.sentList()) != 1 {
		t.Fatalf("second err %v, sends %v, want a retryable wait on the first top-up and no second send",
			err, e.rpc.sentList())
	}
	if got := e.journal(t); !slices.Equal(got, []string{"pending"}) {
		t.Fatalf("journal = %v, want the first top-up still pending", got)
	}
}

func TestTopUpSOL_aTopUpWhoseBlockhashExpiredUnlandedIsSentAgain(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, never{})
	ctx, cancel := context.WithCancel(t.Context())
	e.rpc.unlanded, e.rpc.onSend = true, cancel
	if err := e.topUps.TopUpSOL(ctx, memberWallet); errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("first err = %v, want upstream_timeout", err)
	}
	e.rpc.unlanded, e.rpc.onSend, e.rpc.expire = false, nil, true
	e.topUps = e.with(instant{})
	e.rpc.statuses = []solana.State{solana.StateNotFound, solana.StateProcessing}

	if err := e.topUps.TopUpSOL(t.Context(), memberWallet); err != nil || len(e.rpc.sentList()) != 2 {
		t.Fatalf("err %v, sends %v, want the expired top-up replaced by a second send", err, e.rpc.sentList())
	}
	if got := e.journal(t); !slices.Equal(got, []string{"expired", "confirmed"}) {
		t.Fatalf("journal = %v, want the first expired and the second confirmed", got)
	}
}

func TestTopUpSOL_capsAWalletAtThreeTopUpsADay(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, instant{})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	for i := range relayer.TopUpCap {
		if err := e.topUps.TopUpSOL(ctx, memberWallet); err != nil {
			t.Fatalf("top-up %d: %v", i+1, err)
		}
		e.rpc.drain(memberWallet)
	}

	err := e.topUps.TopUpSOL(ctx, memberWallet)

	if !errs.Retryable(errs.CodeOf(err)) || len(e.rpc.sentList()) != relayer.TopUpCap {
		t.Fatalf("fourth err %v, sends %d, want a retryable refusal after three", err, len(e.rpc.sentList()))
	}
	alerts := logs.Lines(observability.RelayerTopUpCapped.Name, "level", "wallet")
	if len(alerts) != 1 || alerts[0] != "ERROR "+string(memberWallet) {
		t.Fatalf("cap alerts = %q, want one for the wallet", alerts)
	}
}

func TestTopUpSOL_reportsJournalWritesItCannotMake(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		arrange func(t *testing.T, e *topUpEnv)
		sends   int
	}{
		"pending undecodable": {arrange: func(t *testing.T, e *topUpEnv) {
			t.Helper()
			e.exec(t, `INSERT INTO relayer_sol_topups (id, wallet, lamports, signature, last_valid_block_height,
				status, created_at) VALUES (gen_random_uuid(), $1, 1, 'odd', 99999999999999999999, 'pending', now())`,
				string(memberWallet))
		}},
		"pending status unreadable": {arrange: func(t *testing.T, e *topUpEnv) {
			t.Helper()
			e.exec(t, `INSERT INTO relayer_sol_topups (id, wallet, lamports, signature, last_valid_block_height,
				status, created_at) VALUES (gen_random_uuid(), $1, 1, 'unread', 100, 'pending', now())`,
				string(memberWallet))
			e.rpc.Fail("SignatureStatuses", errs.New(errs.CodeRPCUnavailable, "solana"))
		}},
		"journal unreadable": {arrange: func(t *testing.T, e *topUpEnv) {
			t.Helper()
			e.exec(t, `ALTER TABLE relayer_sol_topups RENAME TO relayer_sol_topups_gone`)
		}},
		"insert refused": {arrange: func(t *testing.T, e *topUpEnv) {
			t.Helper()
			e.exec(t, `ALTER TABLE relayer_sol_topups ADD CONSTRAINT refuse CHECK (lamports < 0)`)
		}},
		"count unreadable":   {arrange: func(_ *testing.T, e *topUpEnv) { e.topUps = e.with(&brokenAfter{n: 1}) }},
		"landing unrecorded": {arrange: func(_ *testing.T, e *topUpEnv) { e.topUps = e.with(&brokenAfter{n: 3}) }, sends: 1},
		"pending unresolvable": {arrange: func(t *testing.T, e *topUpEnv) {
			t.Helper()
			e.exec(t, `INSERT INTO relayer_sol_topups (id, wallet, lamports, signature, last_valid_block_height,
				status, created_at) VALUES (gen_random_uuid(), $1, 1, 'gone', 100, 'pending', now())`, string(memberWallet))
			e.rpc.expire = true
			e.topUps = e.with(&brokenAfter{n: 1})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newTopUpEnv(t, 0, funded, instant{})
			c.arrange(t, e)
			if err := e.topUps.TopUpSOL(t.Context(), memberWallet); err == nil || len(e.rpc.sentList()) != c.sends {
				t.Fatalf("err %v, sends %v, want the journal error and %d sends", err, e.rpc.sentList(), c.sends)
			}
		})
	}
}

func TestTopUpSOL_aLockItCannotTakeIsReported(t *testing.T) {
	t.Parallel()
	e := newTopUpEnv(t, 0, funded, instant{})
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = e.uow.Do(context.WithoutCancel(t.Context()), func(ctx context.Context, tx db.Tx) error {
			_, err := tx.Queries().Exec(ctx,
				`SELECT pg_advisory_xact_lock(hashtextextended('sol-topup:' || $1::text, 0))`, string(memberWallet))
			close(held)
			<-release
			return err
		})
	}()
	<-held
	defer close(release)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	if err := e.topUps.TopUpSOL(ctx, memberWallet); err == nil || len(e.rpc.sentList()) != 0 {
		t.Fatalf("err %v, sends %v, want the lock wait reported and nothing sent", err, e.rpc.sentList())
	}
}

func TestLazyTopUps_buildsFromConfigOnFirstUse(t *testing.T) {
	t.Parallel()
	uow := db.New(testkit.DB(t), testkit.NewIDs(2), clock.Real{})
	broken := keyConfig("relayer")
	broken.Relayer.PrivateKey = "not-a-key"
	for range 2 {
		err := relayer.NewLazyTopUps(broken, uow, clock.Real{}).TopUpSOL(t.Context(), memberWallet)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("TopUpSOL with a bad relayer key err = %v, want invalid_input", err)
		}
	}
	lazy := relayer.NewLazyTopUps(keyConfig("relayer"), uow, clock.Real{})
	if err := lazy.TopUpSOL(t.Context(), "not-an-address"); errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("TopUpSOL err = %v, want the built top-ups to refuse the address", err)
	}
}
