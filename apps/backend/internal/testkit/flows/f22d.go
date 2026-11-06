package flows

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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

func SettledProposal(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("f22d-settled-"+name))
}

func settledSeed(s *scenario.Scenario, proposal uuid.UUID, withCard bool) {
	s.Helper()
	_, err := s.DB().Exec(s.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at, updated_at)
		SELECT $1, 'proposal', 'proposals', $2, c.id, c.name, c.creator_id, 'NVDA', 'seeded',
			'{"status":"open","action":"buy","symbol":"NVDA","usdc_micros":"300000000"}', 'open', now(), now()
		FROM cabals c WHERE c.id = $3::uuid`, ids.Real{}.NewV7(), proposal, s.Recall("cabal"))
	if err != nil {
		s.Fatalf("flows: seed the proposal feed item: %v", err)
	}
	if !withCard {
		return
	}
	_, err = s.DB().Exec(s.Context(), `INSERT INTO cabal_messages
		(id, cabal_id, author_id, kind, proposal_id, dedupe_key, created_at)
		VALUES ($1, $2::uuid, $3, 'proposal', $4, $5, now())`,
		ids.Real{}.NewV7(), s.Recall("cabal"), s.ActorID().UUID(), proposal, "proposal:"+proposal.String())
	if err != nil {
		s.Fatalf("flows: seed the proposal card: %v", err)
	}
}

func settledKey(proposal uuid.UUID) string { return "settled:" + proposal.String() }

func expectSettledMoment(outcome string, yes, no int, proposer func(*scenario.Scenario) string) scenario.Step {
	return scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
		var rows []struct {
			Moment struct {
				Kind     string `json:"kind"`
				Outcome  string `json:"outcome"`
				Symbol   string `json:"symbol"`
				Micros   int64  `json:"usdc_micros"`
				Yes      int    `json:"yes"`
				No       int    `json:"no"`
				Voters   int    `json:"voters"`
				Proposer struct {
					ID string `json:"id"`
				} `json:"proposer"`
			} `json:"moment"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			s.Fatalf("flows: decode the moments read: %v", err)
		}
		for _, row := range rows {
			m := row.Moment
			if m.Kind != "proposal_settled" {
				continue
			}
			if m.Outcome != outcome || m.Symbol != "NVDA" || m.Micros != 300_000_000 || m.Yes != yes || m.No != no ||
				m.Voters != 6 || m.Proposer.ID != proposer(s) {
				s.Fatalf(
					"flows: the settled moment reads %+v, want %s %d to %d by %s",
					m,
					outcome,
					yes,
					no,
					proposer(s),
				)
			}
			return
		}
		s.Fatalf("flows: the moments read %s holds no proposal_settled row", raw)
	})
}

func settledRun(
	script, name string, event func(cabal, proposal uuid.UUID) events.Event, withCard bool, outcome string,
	proposer func(*scenario.Scenario) string,
) func(*scenario.Scenario) {
	proposal := SettledProposal(name)
	return func(s *scenario.Scenario) {
		s.Given(chatCabal(script)...).
			When(
				func(s *scenario.Scenario) {
					settledSeed(s, proposal, withCard)
					for range 2 {
						publishEvent(s, event(uuid.MustParse(s.Recall("cabal")), proposal))
					}
				},
				scenario.Eventually("the proposal_settled row", func(s *scenario.Scenario) bool {
					return keyedRows(s, settledKey(proposal)) == 1
				}),
				scenario.Get(chatPath),
				scenario.ExpectStatus(http.StatusOK),
				scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
					if strings.Contains(string(raw), `"kind":"system"`) {
						s.Fatalf("flows: the default read returned %s, want no system rows", raw)
					}
				}),
				scenario.Get(chatPath+"?moments=true"),
				scenario.ExpectStatus(http.StatusOK),
				expectSettledMoment(outcome, 2, 4, proposer),
			).
			Then(
				func(s *scenario.Scenario) {
					if got := keyedRows(s, settledKey(proposal)); got != 1 {
						s.Fatalf("flows: the cabal holds %d rows for the settled key, want 1", got)
					}
				},
				scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
			)
	}
}

func cabalCreator(s *scenario.Scenario) string {
	s.Helper()
	var creator string
	if err := s.DB().QueryRow(s.Context(), `SELECT creator_id::text FROM cabals WHERE id = $1::uuid`,
		s.Recall("cabal")).Scan(&creator); err != nil {
		s.Fatalf("flows: read the cabal creator: %v", err)
	}
	return creator
}

func F22dWriteProposalFailedMomentOK(s *scenario.Scenario) {
	settledRun("moment-ok", "failed", func(cabal, proposal uuid.UUID) events.Event {
		return events.ProposalFailed{V: 1, ProposalID: proposal, CabalID: cabal, Kind: "buy"}
	}, true, "failed", func(s *scenario.Scenario) string { return s.ActorID().UUID().String() })(s)
}

func F22dWriteProposalExpiredMomentOK(s *scenario.Scenario) {
	settledRun("moment-created", "expired", func(cabal, proposal uuid.UUID) events.Event {
		return events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal, Kind: "buy"}
	}, false, "expired", cabalCreator)(s)
}

func F22dWriteProposalFailedMomentSilent(s *scenario.Scenario) {
	rule, voided, withdrawn, unknown, last := SettledProposal("rule"), SettledProposal("voided"),
		SettledProposal("withdrawn"), SettledProposal("unknown"), SettledProposal("last")
	s.Given(chatCabal("moment-invalid")...).
		When(
			func(s *scenario.Scenario) {
				cabal := uuid.MustParse(s.Recall("cabal"))
				for _, p := range []uuid.UUID{rule, voided, withdrawn, last} {
					settledSeed(s, p, false)
				}
				publishEvent(s, events.ProposalFailed{V: 1, ProposalID: rule, CabalID: cabal, Kind: "rule"})
				publishEvent(
					s,
					events.ProposalVoided{V: 1, ProposalID: voided, CabalID: cabal, ActorType: "admin", Reason: "x"},
				)
				publishEvent(s, events.ProposalWithdrawn{V: 1, ProposalID: withdrawn, CabalID: cabal})
				publishEvent(s, events.ProposalFailed{V: 1, ProposalID: unknown, CabalID: cabal, Kind: "buy"})
				publishEvent(s, events.ProposalExpired{V: 1, ProposalID: last, CabalID: cabal, Kind: "buy"})
			},
			scenario.Eventually("the last settled row", func(s *scenario.Scenario) bool {
				return keyedRows(s, settledKey(last)) == 1
			}),
		).
		Then(func(s *scenario.Scenario) {
			if got := keyedRows(s, "settled:%"); got != 1 {
				s.Fatalf("flows: the cabal holds %d settled rows, want only the last proposal's", got)
			}
		})
}

func (defined) AloneF22d() []Script {
	return []Script{F22dWriteTradeFilledMomentOK}
}

func (defined) LettersF22d() map[string][]Letter {
	unbooked := Letter{Consumer: "treasury_activity", Subject: events.TypeTradeConfirmed, Code: errs.CodeInvalidInput}
	return map[string][]Letter{"F22dWriteTradeFilledMomentOK": {unbooked}}
}
