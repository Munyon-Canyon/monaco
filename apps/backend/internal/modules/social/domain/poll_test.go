package domain_test

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestParsePoll_trimsTheTextAndDefaultsToADay(t *testing.T) {
	t.Parallel()
	poll, err := domain.ParsePoll("  Trim?  ", []string{" Yes ", "Wait"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if poll.Question != "Trim?" || !slices.Equal(poll.Options, []string{"Yes", "Wait"}) ||
		poll.ClosesIn != 24*time.Hour {
		t.Fatalf("poll = %+v, want trimmed text and 24 hours", poll)
	}
	week := 7 * 24 * time.Hour
	if poll, err = domain.ParsePoll("Q", []string{"a", "b", "c", "d"}, &week); err != nil || poll.ClosesIn != week {
		t.Fatalf("poll = %+v, err %v, want the 7 day poll with 4 options", poll, err)
	}
}

func TestParsePoll_refusesWithAReason(t *testing.T) {
	t.Parallel()
	tooSoon, tooLate := 30*time.Minute, 8*24*time.Hour
	tests := map[string]struct {
		question string
		options  []string
		closes   *time.Duration
		reason   string
	}{
		"no question":     {"  ", []string{"a", "b"}, nil, "poll_question"},
		"a long question": {strings.Repeat("q", 101), []string{"a", "b"}, nil, "poll_question"},
		"one option":      {"Q", []string{"a"}, nil, "poll_options"},
		"five options":    {"Q", []string{"a", "b", "c", "d", "e"}, nil, "poll_options"},
		"a blank option":  {"Q", []string{"a", " "}, nil, "poll_options"},
		"a long option":   {"Q", []string{"a", strings.Repeat("o", 41)}, nil, "poll_options"},
		"duplicates":      {"Q", []string{"Yes", " YES"}, nil, "poll_options"},
		"closes too soon": {"Q", []string{"a", "b"}, &tooSoon, "poll_close"},
		"closes too late": {"Q", []string{"a", "b"}, &tooLate, "poll_close"},
	}
	for name, tt := range tests {
		_, err := domain.ParsePoll(tt.question, tt.options, tt.closes)
		if errs.CodeOf(err) != errs.CodeInvalidInput || !slices.ContainsFunc(errs.Detail(err), func(a slog.Attr) bool {
			return a.Key == "reason" && a.Value.String() == tt.reason
		}) {
			t.Errorf("%s: error = %v, want invalid_input %s", name, err, tt.reason)
		}
	}
}

func TestPollAttachment_roundTripsItsPayloadAndNamesTheQuestionAsText(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	in, err := domain.ParseCommentInput(nil, &domain.AttachmentInput{
		Kind: "poll", Poll: &domain.PollInput{Question: "Trim?", Options: []string{"Yes", "No"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if in.Text() != "Trim?" {
		t.Fatalf("text = %q, want the question", in.Text())
	}
	got := domain.DecodeAttachment("poll", in.Attachment.Payload(now))
	if got == nil || got.Poll == nil || got.Poll.Question != "Trim?" ||
		!got.Poll.ClosesAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("decoded = %+v, want the question and a close time a day on", got)
	}
	if domain.DecodeAttachment("poll", []byte(`{"closes_at":1}`)) != nil {
		t.Fatal("a poll payload with a bad close time decoded")
	}
	_, err = domain.ParseCommentInput(nil, &domain.AttachmentInput{
		Kind: "poll", Poll: &domain.PollInput{Question: "Q", Options: []string{"only"}},
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("one option poll error = %v, want invalid_input", err)
	}
}

func TestParsePollPosition_acceptsZeroToThree(t *testing.T) {
	t.Parallel()
	for _, n := range []int{-1, 4} {
		if _, err := domain.ParsePollPosition(n); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("position %d: error = %v, want invalid_input", n, err)
		}
	}
	if got, err := domain.ParsePollPosition(3); err != nil || got != 3 {
		t.Fatalf("position 3 = %d, %v, want 3", got, err)
	}
}

func TestParseCommentInput_refusesAPollKindWithNoPoll(t *testing.T) {
	t.Parallel()
	_, err := domain.ParseCommentInput(nil, &domain.AttachmentInput{Kind: "poll"})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}
