package domain

import (
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	AttachmentPoll AttachmentKind = "poll"

	DefaultPollClosesIn = 24 * time.Hour
	MinPollClosesIn     = time.Hour
	MaxPollClosesIn     = 7 * 24 * time.Hour

	pollQuestionMax = 100
	pollOptionMax   = 40
	pollOptionsMin  = 2
	pollOptionsMax  = 4
)

type Poll struct {
	Question string
	Options  []string
	ClosesIn time.Duration
	ClosesAt time.Time
}

type PollInput struct {
	Question string
	Options  []string
	ClosesIn *time.Duration
}

func ParsePoll(question string, options []string, closesIn *time.Duration) (Poll, error) {
	const op = "social.ParsePoll"
	invalid := func(reason string) error {
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", reason))
	}
	question = strings.TrimSpace(question)
	if n := utf8.RuneCountInString(question); n < 1 || n > pollQuestionMax {
		return Poll{}, invalid("poll_question")
	}
	if len(options) < pollOptionsMin || len(options) > pollOptionsMax {
		return Poll{}, invalid("poll_options")
	}
	seen := map[string]bool{}
	labels := make([]string, len(options))
	for i, raw := range options {
		label := strings.TrimSpace(raw)
		key := strings.ToLower(label)
		if n := utf8.RuneCountInString(label); n < 1 || n > pollOptionMax || seen[key] {
			return Poll{}, invalid("poll_options")
		}
		seen[key] = true
		labels[i] = label
	}
	in := DefaultPollClosesIn
	if closesIn != nil {
		in = *closesIn
	}
	if in < MinPollClosesIn || in > MaxPollClosesIn {
		return Poll{}, invalid("poll_close")
	}
	return Poll{Question: question, Options: labels, ClosesIn: in}, nil
}

func ParsePollPosition(position int) (int16, error) {
	if position < 0 || position >= pollOptionsMax {
		return 0, errs.New(errs.CodeInvalidInput, "social.ParsePollPosition", slog.String("reason", "poll_option"))
	}
	return int16(position), nil
}
