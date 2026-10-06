package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const RealtimeTokenTTL = 15 * time.Minute

type RealtimeTokenHandler struct {
	members  Members
	realtime Realtime
}

func NewRealtimeTokenHandler(members Members, realtime Realtime) *RealtimeTokenHandler {
	return &RealtimeTokenHandler{members: members, realtime: realtime}
}

func (h *RealtimeTokenHandler) Handle(ctx context.Context, user ids.UserID) (TokenRequest, error) {
	const op = "social.RealtimeToken"
	cabals, err := h.members.CabalsOf(ctx, user)
	if err != nil {
		return TokenRequest{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(cabals) == 0 {
		return TokenRequest{}, errs.New(errs.CodeNoRealtimeChannels, op)
	}
	channels := make([]string, len(cabals))
	for i, cabal := range cabals {
		channels[i] = CabalChannel(cabal)
	}
	return h.realtime.TokenRequest(ctx, user, channels, RealtimeTokenTTL)
}
