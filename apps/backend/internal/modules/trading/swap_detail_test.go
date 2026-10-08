package trading_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/tradingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func (s retryServer) get(t *testing.T, swap uuid.UUID, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/swaps/"+swap.String(), http.NoBody)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s retryServer) swap(t *testing.T, swap uuid.UUID) api.SwapDetail {
	t.Helper()
	rec := s.get(t, swap, s.token())
	if rec.Code != http.StatusOK {
		t.Fatalf("GET swap %s = %d %s, want 200", swap, rec.Code, rec.Body)
	}
	var out api.SwapDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGetSwap_latestFailedBuyIsRetryableWithItsAssetAndMessage(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	got := s.swap(t, s.failed.ID)
	aaplx := marketfake.AAPLx()
	usdc, sig := int64(25_000_000), "sig-"+s.failed.ID.String()
	want := api.SwapDetail{
		Id: s.failed.ID, CabalId: s.failed.CabalID, Source: api.SwapSource{Kind: "proposal", Id: s.failed.SourceID},
		Action: api.Buy, Symbol: "AAPLx", AssetName: aaplx.DisplayName, TokenDecimals: int(aaplx.Decimals),
		UsdcMicros: &usdc, Status: "failed", FailureCode: ptrTo("jupiter_failed"),
		FailureMessage: ptrTo(errs.Message(errs.CodeSwapFailed)), TxSignature: &sig, CreatedAt: got.CreatedAt,
		Retryable: true,
	}
	if !got.CreatedAt.Equal(s.now) || !reflect.DeepEqual(got, want) {
		t.Fatalf("swap = %+v, want %+v created at %s", got, want, s.now)
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestGetSwap_confirmedSellSwapsTheAmountsAndIsNotRetryable(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	sell := s.created(s.ids.NewV7(), aaplxMint)
	sell.CabalID, sell.Action, sell.OutMint, sell.InAmount = s.failed.CabalID, "sell", usdcMint, 50_000_000
	s.insert(t, sell)
	s.submit(t, sell.ID, "req-sell", "sig-sell")
	s.confirm(t, sell.ID)

	got := s.swap(t, sell.ID)
	if got.Action != api.Sell || got.UsdcMicros == nil || *got.UsdcMicros != 104_900_000 ||
		got.TokenAmount == nil || *got.TokenAmount != 50_000_000 || got.Status != "confirmed" ||
		got.FailureCode != nil || got.FailureMessage != nil || got.ConfirmedAt == nil || got.Retryable {
		t.Fatalf("swap = %+v, want a confirmed sell of 50_000_000 units for 104.9 USDC, not retryable", got)
	}
}

func TestGetSwap_failedSwapWithALaterSwapIsNotRetryable(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	later := s.created(s.failed.SourceID, usdcMint)
	later.CabalID, later.CreatedAt = s.failed.CabalID, s.now.Add(time.Second)
	s.insert(t, later)
	if got := s.swap(t, s.failed.ID); got.Retryable {
		t.Fatal("failed swap still retryable after a later swap, want false")
	}
	if got := s.swap(t, later.ID); got.Status != "created" || got.TxSignature != nil || got.Retryable {
		t.Fatalf("later swap = %+v, want created with no signature and not retryable", got)
	}
}

func TestGetSwap_failedCashoutLegIsNotRetryable(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	cashout := s.failedSwap(t, s.ids.NewV7(), "cashout")
	if got := s.swap(t, cashout.ID); got.Retryable || got.Source.Kind != "cashout" {
		t.Fatalf("swap = %+v, want a cashout leg that is not retryable", got)
	}
}

func TestGetSwap_refusals(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	stranger := s.verifier.Mint(s.ids.NewV7().String(), s.clock.Now().Add(time.Hour))
	cases := map[string]struct {
		swap  uuid.UUID
		token string
		want  int
	}{
		"no token":     {swap: s.failed.ID, want: http.StatusUnauthorized},
		"unknown swap": {swap: s.ids.NewV7(), token: s.token(), want: http.StatusNotFound},
		"not a member": {swap: s.failed.ID, token: stranger, want: http.StatusForbidden},
	}
	for name, tc := range cases {
		if rec := s.get(t, tc.swap, tc.token); rec.Code != tc.want {
			t.Errorf("%s: status = %d %s, want %d", name, rec.Code, rec.Body, tc.want)
		}
	}
}

func TestGetSwap_nonMemberReadsOnlyAConfirmedProposalTrade(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	stranger := s.verifier.Mint(s.ids.NewV7().String(), s.clock.Now().Add(time.Hour))
	created := func(kind string) sqlc.InsertCreatedParams {
		row := s.created(s.ids.NewV7(), usdcMint)
		row.SourceKind, row.CabalID = kind, s.failed.CabalID
		s.insert(t, row)
		return row
	}
	confirmed := func(kind string) uuid.UUID {
		row := created(kind)
		s.submit(t, row.ID, "req-"+row.ID.String(), "sig-"+row.ID.String())
		s.confirm(t, row.ID)
		return row.ID
	}
	pending, cashout, proposal := created("proposal").ID, confirmed("cashout"), confirmed("proposal")
	cases := map[string]struct {
		swap  uuid.UUID
		token string
		want  int
	}{
		"member reads a failed swap":             {swap: s.failed.ID, token: s.token(), want: http.StatusOK},
		"member reads a confirmed cashout":       {swap: cashout, token: s.token(), want: http.StatusOK},
		"stranger reads a confirmed proposal":    {swap: proposal, token: stranger, want: http.StatusOK},
		"stranger refused a pending proposal":    {swap: pending, token: stranger, want: http.StatusForbidden},
		"stranger refused a failed proposal":     {swap: s.failed.ID, token: stranger, want: http.StatusForbidden},
		"stranger refused a confirmed cashout":   {swap: cashout, token: stranger, want: http.StatusForbidden},
		"stranger gets not found for unknown id": {swap: s.ids.NewV7(), token: stranger, want: http.StatusNotFound},
	}
	for name, tc := range cases {
		if rec := s.get(t, tc.swap, tc.token); rec.Code != tc.want {
			t.Errorf("%s: status = %d %s, want %d", name, rec.Code, rec.Body, tc.want)
		}
	}
}

func TestSwapDetailReads_failures(t *testing.T) {
	t.Parallel()
	setMint := func(t *testing.T, e *retryEnv, mint string) {
		t.Helper()
		_, err := e.pool.Exec(t.Context(), `UPDATE swaps SET out_mint = $2 WHERE id = $1`, e.failed.ID, mint)
		if err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]struct {
		arrange func(t *testing.T, e *retryEnv) context.Context
		want    errs.Code
	}{
		"lookup fails": {
			arrange: func(t *testing.T, _ *retryEnv) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			want: errs.CodeInternal,
		},
		"cabal port down": {
			arrange: func(t *testing.T, e *retryEnv) context.Context {
				t.Helper()
				e.cabals.Fail("IsMember", errs.New(errs.CodeUpstreamUnavailable, "test"))
				return t.Context()
			},
			want: errs.CodeUpstreamUnavailable,
		},
		"stored mint is not an address": {
			arrange: func(t *testing.T, e *retryEnv) context.Context {
				t.Helper()
				setMint(t, e, "not-a-mint")
				return t.Context()
			},
			want: errs.CodeDecodeFailed,
		},
		"mint missing from the catalog": {
			arrange: func(t *testing.T, e *retryEnv) context.Context {
				t.Helper()
				setMint(t, e, "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")
				return t.Context()
			},
			want: errs.CodeAssetNotFound,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRetryEnv(t)
			ctx := tc.arrange(t, e)
			reads := app.NewSwapDetailReads(e.pool, e.cabals, marketfake.NewCatalog(marketfake.Fixtures()...))
			if _, err := reads.Swap(ctx, ids.SwapIDFrom(e.failed.ID), e.member); errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}
