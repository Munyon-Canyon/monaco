package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type FollowSource string

const (
	SourceProfile   FollowSource = "profile"
	SourcePhone     FollowSource = "phone"
	SourceX         FollowSource = "x"
	SourceCabal     FollowSource = "cabal"
	SourceFeed      FollowSource = "feed"
	SourceSuggested FollowSource = "suggested"
	SourceReferral  FollowSource = "referral"
)

func clientSources() []FollowSource {
	return []FollowSource{SourceProfile, SourcePhone, SourceX, SourceCabal, SourceFeed, SourceSuggested}
}

func ParseClientSource(raw string) (FollowSource, error) {
	source := FollowSource(raw)
	switch {
	case source == "":
		return SourceProfile, nil
	case slices.Contains(clientSources(), source):
		return source, nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "social.ParseClientSource", slog.String("source", raw))
	}
}

func (s FollowSource) String() string { return string(s) }
