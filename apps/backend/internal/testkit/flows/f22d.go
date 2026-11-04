package flows

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
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

func joinedKey(s *scenario.Scenario, request uuid.UUID) string {
	return "joined:" + s.ActorID().String() + ":" + request.String()
}

func F22dWriteMemberJoinedMomentOK(s *scenario.Scenario) {
	request := uuid.MustParse("01920000-0000-7000-8000-0000000000c2")
	s.Given(chatCabal("moment-ok")...).
		When(
			scenario.Eventually("the cabal_created row", func(s *scenario.Scenario) bool { return keyedRows(s, "created") == 1 }),
			func(s *scenario.Scenario) {
				for range 2 {
					publishEvent(s, events.CabalMemberJoined{
						V: 1, CabalID: uuid.MustParse(s.Recall("cabal")), UserID: s.ActorID().UUID(),
						Role: "member", Via: "open", RequestID: request,
					})
				}
			},
			scenario.Eventually("the member_joined row", func(s *scenario.Scenario) bool {
				return keyedRows(s, joinedKey(s, request)) == 1
			}),
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
			func(s *scenario.Scenario) {
				if got := keyedRows(s, joinedKey(s, request)); got != 1 {
					s.Fatalf("flows: the cabal holds %d rows for the joined key, want 1", got)
				}
			},
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
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
