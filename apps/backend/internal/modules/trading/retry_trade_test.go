package trading_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type retryEnv struct {
	swapDB
	member    ids.UserID
	cabals    *fakes.Cabal
	proposals *proposalsFake
	failed    sqlc.InsertCreatedParams
}

func newRetryEnv(t *testing.T) *retryEnv {
	t.Helper()
	e := &retryEnv{swapDB: newSwapDB(t), proposals: &proposalsFake{status: governance.StatusPassed}}
	e.member = ids.UserIDFrom(e.ids.NewV7())
	e.failed = e.failedSwap(t, e.ids.NewV7(), "proposal")
	e.cabals = fakes.NewCabal(nil, []fakes.CabalMember{{
		CabalID: ids.CabalIDFrom(e.failed.CabalID), Member: cabal.MemberView{UserID: e.member},
	}})
	return e
}

func (e *retryEnv) failedSwap(t *testing.T, source uuid.UUID, kind string) sqlc.InsertCreatedParams {
	t.Helper()
	row := e.created(source, usdcMint)
	row.SourceKind = kind
	if e.failed.ID != uuid.Nil {
		row.CabalID = e.failed.CabalID
	}
	e.insert(t, row)
	e.submit(t, row.ID, "req-"+row.ID.String(), "sig-"+row.ID.String())
	e.fail(t, row.ID, string(domain.FailureJupiterFailed))
	return row
}

func (e *retryEnv) retry(ctx context.Context, swap uuid.UUID) error {
	uow := db.New(e.pool, e.ids, testkit.NewClock(e.now))
	return app.NewRetryTradeHandler(uow, e.pool, e.cabals, e.proposals).
		Handle(ctx, app.RetryTrade{SwapID: ids.SwapIDFrom(swap), ActorID: e.member})
}

func (e *retryEnv) requests(t *testing.T, swap uuid.UUID) []busevents.TradeRetryRequested {
	t.Helper()
	rows, err := e.pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = 'trade.retry_requested' AND aggregate_id = $1 ORDER BY id`, swap)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []busevents.TradeRetryRequested
	for rows.Next() {
		var raw []byte
		var ev busevents.TradeRetryRequested
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func memberContext(ctx context.Context, user ids.UserID) context.Context {
	return observability.WithActor(ctx, "user:"+user.String())
}

func TestRetryTrade_appendsTheFailedSwapsParameters(t *testing.T) {
	t.Parallel()
	e := newRetryEnv(t)
	if err := e.retry(memberContext(t.Context(), e.member), e.failed.ID); err != nil {
		t.Fatal(err)
	}
	got := e.requests(t, e.failed.ID)
	want := []busevents.TradeRetryRequested{{
		V: 1, SwapID: e.failed.ID, CabalID: e.failed.CabalID,
		Source: busevents.TradeSource{Kind: "proposal", ID: e.failed.SourceID}, Action: "buy", Symbol: "AAPLx",
		InMint: usdcMint, OutMint: aaplxMint, InAmount: 25_000_000, QuoteOutAmount: 105_000_000, SlippageBps: 100,
		RequestedBy: e.member.UUID(),
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("trade.retry_requested = %+v, want %+v", got, want)
	}
}

func TestRetryTrade_refusesBeforeAppending(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		arrange func(t *testing.T, e *retryEnv) uuid.UUID
		want    errs.Code
	}{
		"unknown swap": {
			arrange: func(_ *testing.T, e *retryEnv) uuid.UUID { return e.ids.NewV7() },
			want:    errs.CodeSwapNotFound,
		},
		"caller is not a member": {
			arrange: func(_ *testing.T, e *retryEnv) uuid.UUID {
				e.member = ids.UserIDFrom(e.ids.NewV7())
				return e.failed.ID
			},
			want: errs.CodeNotCabalMember,
		},
		"proposal is no longer passed": {
			arrange: func(_ *testing.T, e *retryEnv) uuid.UUID {
				e.proposals.set(governance.Status("execution_blocked"))
				return e.failed.ID
			},
			want: errs.CodeSwapNotRetryable,
		},
		"a later swap superseded it": {
			arrange: func(t *testing.T, e *retryEnv) uuid.UUID {
				t.Helper()
				e.now = e.now.Add(1)
				e.failedSwap(t, e.failed.SourceID, "proposal")
				return e.failed.ID
			},
			want: errs.CodeSwapNotRetryable,
		},
		"negative stored amount": {
			arrange: func(t *testing.T, e *retryEnv) uuid.UUID {
				t.Helper()
				if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET in_amount = -1 WHERE id = $1`,
					e.failed.ID); err != nil {
					t.Fatal(err)
				}
				return e.failed.ID
			},
			want: errs.CodeDecodeFailed,
		},
		"cabal port down": {
			arrange: func(_ *testing.T, e *retryEnv) uuid.UUID {
				e.cabals.Fail("IsMember", errs.New(errs.CodeUpstreamUnavailable, "test"))
				return e.failed.ID
			},
			want: errs.CodeUpstreamUnavailable,
		},
		"governance port down": {
			arrange: func(_ *testing.T, e *retryEnv) uuid.UUID {
				e.proposals.Fail("Status", errs.New(errs.CodeUpstreamUnavailable, "test"))
				return e.failed.ID
			},
			want: errs.CodeUpstreamUnavailable,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRetryEnv(t)
			swap := tc.arrange(t, e)
			err := e.retry(memberContext(t.Context(), e.member), swap)
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
			if n := len(e.requests(t, swap)); n != 0 {
				t.Fatalf("appended %d trade.retry_requested, want none", n)
			}
		})
	}
}

func TestRetryTrade_LiveSwap_NotRetryable(t *testing.T) {
	t.Parallel()
	e := newRetryEnv(t)
	live := e.created(e.failed.SourceID, usdcMint)
	live.CabalID, live.CreatedAt = e.failed.CabalID, e.now.Add(1)
	e.insert(t, live)
	e.submit(t, live.ID, "req-live", "sig-live")
	for _, swap := range []uuid.UUID{e.failed.ID, live.ID} {
		if err := e.retry(memberContext(t.Context(), e.member), swap); errs.CodeOf(err) != errs.CodeSwapNotRetryable {
			t.Fatalf("retry %s err = %v, want swap_not_retryable", swap, err)
		}
	}
}

func TestRetryTrade_CashoutSource_NotRetryable(t *testing.T) {
	t.Parallel()
	e := newRetryEnv(t)
	cashout := e.failedSwap(t, e.ids.NewV7(), "cashout")
	if err := e.retry(memberContext(t.Context(), e.member), cashout.ID); errs.CodeOf(err) != errs.CodeSwapNotRetryable {
		t.Fatalf("err = %v, want swap_not_retryable", err)
	}
}

func TestRetryTrade_failedLookupIsInternal(t *testing.T) {
	t.Parallel()
	e := newRetryEnv(t)
	ctx, cancel := context.WithCancel(memberContext(t.Context(), e.member))
	cancel()
	if err := e.retry(ctx, e.failed.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestRetryTrade_failedAppendRollsBack(t *testing.T) {
	t.Parallel()
	e := newRetryEnv(t)
	if err := e.retry(t.Context(), e.failed.ID); err == nil {
		t.Fatal("retry without an actor succeeded, want the append to fail")
	}
	if n := len(e.requests(t, e.failed.ID)); n != 0 {
		t.Fatalf("appended %d trade.retry_requested, want none", n)
	}
}
