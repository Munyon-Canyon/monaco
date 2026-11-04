package flows

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const proposalThesis = "put it up, small add before the print"

type fixedHeads map[ids.ProposalID]governanceport.ProposalHead

func (h fixedHeads) ProposalHead(_ context.Context, id ids.ProposalID) (governanceport.ProposalHead, error) {
	head, ok := h[id]
	if !ok {
		return governanceport.ProposalHead{}, errs.New(errs.CodeProposalNotFound, "flows.fixedHeads")
	}
	return head, nil
}

type proposalWriter struct {
	handler  *app.WriteChatProposalMessage
	rt       *momentRealtime
	proposal uuid.UUID
}

func newProposalWriter(s *scenario.Scenario, known bool) proposalWriter {
	rt := &momentRealtime{}
	wire := func(_ context.Context, m app.ChatMessage) (any, error) { return map[string]any{"id": m.ID}, nil }
	proposal := uuid.New()
	heads := fixedHeads{}
	if known {
		heads[ids.ProposalIDFrom(proposal)] = governanceport.ProposalHead{
			Kind:   "buy",
			Symbol: "NVDA",
			Thesis: proposalThesis,
		}
	}
	return proposalWriter{
		rt: rt, proposal: proposal,
		handler: app.NewWriteChatProposalMessage(app.ChatDeps{
			UoW: db.New(s.DB(), ids.Real{}, clock.Real{}), IDs: ids.Real{}, Clock: clock.Real{},
			Publish: app.NewChatPublisher(rt, wire),
		}, heads),
	}
}

func proposalCtx(s *scenario.Scenario) context.Context {
	return auth.WithActor(s.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "consumer:proposal.created"})
}

func (w proposalWriter) event(s *scenario.Scenario) events.ProposalCreated {
	return events.ProposalCreated{
		V: 1, ProposalID: w.proposal, CabalID: uuid.MustParse(s.Recall("cabal")), ProposerID: s.ActorID().UUID(),
		Kind: "buy", Symbol: "NVDA", ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
}

func proposalRowsHold(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1::uuid AND kind = 'proposal'
			 AND proposal_id IS NOT NULL AND author_id IS NOT NULL`, s.Recall("cabal")).Scan(&got)
		if err != nil {
			s.Fatalf("flows: count the proposal rows: %v", err)
		}
		if got != want {
			s.Fatalf("flows: the cabal holds %d proposal rows, want %d", got, want)
		}
	}
}

func proposalsPublished(w *proposalWriter, want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if got := w.rt.published(); got != want {
			s.Fatalf("flows: %d message.created published, want %d", got, want)
		}
	}
}

func F22eWriteChatProposalMessageOK(s *scenario.Scenario) {
	var w proposalWriter
	s.Given(append(chatCabal("proposal-ok"), func(s *scenario.Scenario) { w = newProposalWriter(s, true) })...).
		When(
			func(s *scenario.Scenario) {
				for range 2 {
					if err := w.handler.Handle(proposalCtx(s), w.event(s)); err != nil {
						s.Fatalf("flows: write the chat proposal message: %v", err)
					}
				}
			},
			scenario.Get(chatPath),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectField("messages", func(s *scenario.Scenario, raw json.RawMessage) {
				var got []struct {
					Kind       string  `json:"kind"`
					ProposalID string  `json:"proposal_id"`
					Body       *string `json:"body"`
					Author     struct {
						ID string `json:"id"`
					} `json:"author"`
				}
				if err := json.Unmarshal(raw, &got); err != nil {
					s.Fatalf("flows: decode the messages %s: %v", raw, err)
				}
				if len(got) != 1 || got[0].Kind != "proposal" || got[0].ProposalID != w.proposal.String() ||
					got[0].Body == nil || *got[0].Body != proposalThesis || got[0].Author.ID != s.ActorID().String() {
					s.Fatalf("flows: the channel read %s, want one proposal row by the proposer", raw)
				}
			}),
		).
		Then(
			proposalRowsHold(1),
			func(s *scenario.Scenario) { proposalsPublished(&w, 1)(s) },
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22eWriteChatProposalMessageProposalNotFound(s *scenario.Scenario) {
	var w proposalWriter
	s.Given(append(chatCabal("proposal-missing"), func(s *scenario.Scenario) { w = newProposalWriter(s, false) })...).
		When(func(s *scenario.Scenario) {
			if err := w.handler.Handle(proposalCtx(s), w.event(s)); err != nil {
				s.Fatalf("flows: a proposal that is gone must be acknowledged, got %v", err)
			}
		}).
		Then(
			proposalRowsHold(0),
			func(s *scenario.Scenario) { proposalsPublished(&w, 0)(s) },
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}

func F22eWriteChatProposalMessageCrashBeforeCommit(s *scenario.Scenario) {
	var w proposalWriter
	s.Given(append(chatCabal("proposal-crash"), func(s *scenario.Scenario) { w = newProposalWriter(s, true) })...).
		When(func(s *scenario.Scenario) {
			if !faultpoint.Enabled {
				s.Fatalf("flows: this script needs -tags faultpoints")
			}
			crashed := func() (p any) {
				defer func() { p = recover() }()
				_ = w.handler.Handle(faultpoint.Armed(proposalCtx(s), faultpoint.BeforeCommit), w.event(s))
				return nil
			}()
			if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
				s.Fatalf("flows: the write returned %v without crashing at before-commit", crashed)
			}
			if err := w.handler.Handle(proposalCtx(s), w.event(s)); err != nil {
				s.Fatalf("flows: the restarted write: %v", err)
			}
		}).
		Then(
			proposalRowsHold(1),
			func(s *scenario.Scenario) { proposalsPublished(&w, 1)(s) },
			scenario.ExpectEvents(events.TypeChatMessagePosted, 0),
		)
}
