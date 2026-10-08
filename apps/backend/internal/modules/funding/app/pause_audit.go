package app

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type pausedReasons struct {
	PausedReasons []string `json:"paused_reasons"`
}

func appendAdminAction(ctx context.Context, tx db.Tx, action events.AdminAction, before, after []string) error {
	action.Before = pausedReasonsJSON(before)
	action.After = pausedReasonsJSON(after)
	return tx.Events.Append(ctx, action)
}

func pausedReasonsJSON(reasons []string) json.RawMessage {
	distinct := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if !slices.Contains(distinct, reason) {
			distinct = append(distinct, reason)
		}
	}
	encoded, _ := json.Marshal(pausedReasons{PausedReasons: distinct})
	return encoded
}
