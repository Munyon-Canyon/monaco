package flows

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type momentRealtime struct {
	mu    sync.Mutex
	names []string
}

func (r *momentRealtime) Publish(_ context.Context, _, name string, _ any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	return nil
}

func (*momentRealtime) TokenRequest(context.Context, ids.UserID, []string, time.Duration) (app.TokenRequest, error) {
	return app.TokenRequest{}, nil
}

func (*momentRealtime) RevokeTokens(context.Context, ids.UserID) error { return nil }

func (r *momentRealtime) published() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.names)
}

type momentWriter struct {
	handler *app.WriteChatMomentHandler
	rt      *momentRealtime
}

func newMomentWriter(s *scenario.Scenario) momentWriter {
	rt := &momentRealtime{}
	wire := func(_ context.Context, m app.ChatMessage) (any, error) { return map[string]any{"id": m.ID}, nil }
	return momentWriter{
		rt: rt,
		handler: app.NewWriteChatMomentHandler(app.ChatDeps{
			UoW: db.New(s.DB(), ids.Real{}, clock.Real{}), IDs: ids.Real{}, Clock: clock.Real{},
			Publish: app.NewChatPublisher(rt, wire),
		}),
	}
}

func momentCtx(s *scenario.Scenario) context.Context {
	return auth.WithActor(s.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "consumer:cabal.created"})
}

func momentCommand(s *scenario.Scenario, user ids.UserID, key string) app.WriteChatMoment {
	moment, err := domain.NewMemberJoinedMoment(user)
	if err != nil {
		s.Fatalf("flows: build the member_joined moment: %v", err)
	}
	return app.WriteChatMoment{
		CabalID: ids.CabalIDFrom(uuid.MustParse(s.Recall("cabal"))), Moment: moment,
		At: time.Now().UTC(), DedupeKey: key,
	}
}

func systemRowsHold(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1::uuid AND kind = 'system'
			 AND author_id IS NULL AND body IS NULL AND dedupe_key LIKE 'member_joined:%'`, s.Recall("cabal")).Scan(&got)
		if err != nil {
			s.Fatalf("flows: count the system rows: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the cabal holds %d system rows, want %d", got, want)
		}
	}
}

func momentsPublished(w *momentWriter, want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if got := w.rt.published(); got != want {
			s.Fatalf("flows: %d moment.created published, want %d", got, want)
		}
	}
}

func F22dWriteMemberJoinedMomentOK(s *scenario.Scenario) {
	var w momentWriter
	s.Given(append(chatCabal("moment-ok"), func(s *scenario.Scenario) { w = newMomentWriter(s) })...).
		When(
			func(s *scenario.Scenario) {
				cmd := momentCommand(s, s.ActorID(), "member_joined:ok")
				for range 2 {
					if _, err := w.handler.Handle(momentCtx(s), cmd); err != nil {
						s.Fatalf("flows: write the chat moment: %v", err)
					}
				}
			},
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
				if string(raw) != "[]" {
					s.Fatalf("flows: the default read returned %s, want no system rows", raw)
				}
			}),
			scenario.Get(chatPath+"?moments=true"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
				if len(raw) < 3 || raw[0] != '[' || raw[1] != '{' {
					s.Fatalf("flows: the moments read returned %s, want the system row", raw)
				}
			}),
		).
		Then(
			systemRowsHold(1),
			func(s *scenario.Scenario) { momentsPublished(&w, 1)(s) },
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22dWriteMemberJoinedMomentInvalidInput(s *scenario.Scenario) {
	s.Given(chatCabal("moment-invalid")...).
		When(
			scenario.Eventually("the cabal_created row", func(s *scenario.Scenario) bool { return keyedRows(s, "created") == 1 }),
			func(s *scenario.Scenario) {
				publishEvent(s, events.CabalMemberJoined{
					V: 1, CabalID: uuid.MustParse(s.Recall("cabal")), UserID: uuid.Nil, Role: "member", Via: "open",
				})
			},
			scenario.EventuallyLog(observability.BusDispatched, map[string]string{"code": string(errs.CodeInvalidInput)}),
		).
		Then(func(s *scenario.Scenario) {
			if got := keyedRows(s, "joined:"+uuid.Nil.String()+":%"); got != 0 {
				s.Fatalf("flows: the cabal holds %d rows for the nil user, want 0", got)
			}
		})
}

func F22dWriteMemberJoinedMomentCrashBeforeCommit(s *scenario.Scenario) {
	request := uuid.MustParse("01920000-0000-7000-8000-0000000000c1")
	var key string
	s.Given(chatCabal("moment-crash")...).
		When(
			scenario.Eventually("the cabal_created row", func(s *scenario.Scenario) bool { return keyedRows(s, "created") == 1 }),
			func(s *scenario.Scenario) {
				key = "joined:" + s.ActorID().String() + ":" + request.String()
				publishEvent(s, events.CabalMemberJoined{
					V: 1, CabalID: uuid.MustParse(s.Recall("cabal")), UserID: s.ActorID().UUID(),
					Role: "member", Via: "open", RequestID: request,
				})
			},
			scenario.PublishCrashingAt(faultpoint.BeforeCommit),
		).
		Then(
			scenario.Eventually("the member_joined row", func(s *scenario.Scenario) bool { return keyedRows(s, key) == 1 }),
			func(s *scenario.Scenario) {
				if got := keyedRows(s, key); got != 1 {
					s.Fatalf("flows: the cabal holds %d rows for %s, want 1", got, key)
				}
			},
		)
}

func keyedRows(s *scenario.Scenario, pattern string) int {
	s.Helper()
	var got int
	err := s.DB().QueryRow(s.Context(),
		`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1::uuid AND kind = 'system' AND dedupe_key LIKE $2`,
		s.Recall("cabal"), pattern).Scan(&got)
	if err != nil {
		s.Fatalf("flows: count the %s rows: %v", pattern, err)
	}
	return got
}

func F22dWriteCabalCreatedMomentOK(s *scenario.Scenario) {
	s.Given(chatCabal("moment-created")...).
		When(scenario.Eventually("the cabal_created and member_joined rows", func(s *scenario.Scenario) bool {
			return keyedRows(s, "created") == 1 && keyedRows(s, "joined:%") >= 1
		})).
		Then(func(s *scenario.Scenario) {
			if got := keyedRows(s, "created"); got != 1 {
				s.Fatalf("flows: the cabal holds %d cabal_created rows, want 1", got)
			}
		})
}

func tradeConfirmed(s *scenario.Scenario, mint chain.SolanaAddress, action string) events.TradeConfirmed {
	units := money.UnitsFromUint64(2 * pow10(marketfake.AAPLx().Decimals))
	e := events.TradeConfirmed{
		V: 1, SwapID: uuid.MustParse("01920000-0000-7000-8000-0000000000a1"),
		CabalID: uuid.MustParse(s.Recall("cabal")), Action: action,
		USDCMicros: money.MicrosFromUint64(393_500_000), ConfirmedAt: time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC),
	}
	if action == "sell" {
		e.InMint, e.InAmount = mint, units
	} else {
		e.OutMint, e.OutAmount = mint, units
	}
	return e
}

func pow10(n uint8) uint64 {
	v := uint64(1)
	for range n {
		v *= 10
	}
	return v
}

func publishEvent(s *scenario.Scenario, e events.Event) {
	s.Helper()
	ctx := observability.WithActor(s.Context(), "system:flows")
	err := db.New(s.DB(), ids.Real{}, clock.Real{}).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, e)
	})
	if err != nil {
		s.Fatalf("flows: append the %s event: %v", e.Type(), err)
	}
}

func F22dWriteTradeFilledMomentOK(s *scenario.Scenario) {
	s.Given(append(chatCabal("moment-fill"), ensureSamplerCatalog())...).
		When(
			func(s *scenario.Scenario) {
				mint := marketfake.AAPLx().Mint.Address()
				for _, action := range []string{"buy", "buy", "swap"} {
					publishEvent(s, tradeConfirmed(s, mint, action))
				}
				sell := tradeConfirmed(s, mint, "sell")
				sell.SwapID = uuid.MustParse("01920000-0000-7000-8000-0000000000a2")
				publishEvent(s, sell)
			},
			scenario.Eventually("the two trade_filled rows", func(s *scenario.Scenario) bool { return keyedRows(s, "fill:%") >= 2 }),
		).
		Then(func(s *scenario.Scenario) {
			if got := keyedRows(s, "fill:%"); got != 2 {
				s.Fatalf("flows: the cabal holds %d trade_filled rows, want 2", got)
			}
		})
}

func F22dWriteTradeFilledMomentAssetNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("moment-fill-missing")...).
		When(
			func(s *scenario.Scenario) {
				unlisted := chain.SolanaAddress("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
				publishEvent(s, tradeConfirmed(s, unlisted, "buy"))
			},
			scenario.EventuallyLog(observability.BusDispatched, map[string]string{"code": string(errs.CodeAssetNotFound)}),
		).
		Then(func(s *scenario.Scenario) {
			if got := keyedRows(s, "fill:%"); got != 0 {
				s.Fatalf("flows: the cabal holds %d trade_filled rows, want 0", got)
			}
		})
}

func (defined) AloneF22d() []Script {
	return []Script{F22dWriteMemberJoinedMomentInvalidInput, F22dWriteTradeFilledMomentAssetNotFound}
}

func (defined) LettersF22d() map[string][]Letter {
	unlisted := Letter{Subject: events.TypeTradeConfirmed, Code: errs.CodeAssetNotFound}
	invalidJoin := Letter{Subject: events.TypeCabalMemberJoined, Code: errs.CodeInvalidInput}
	unbooked := Letter{Subject: events.TypeTradeConfirmed, Code: errs.CodeInvalidInput}
	return map[string][]Letter{
		"F22dWriteMemberJoinedMomentInvalidInput": {invalidJoin},
		"F22dWriteTradeFilledMomentAssetNotFound": {unlisted, unbooked},
	}
}
