package app

import (
	"context"
	"encoding/json"

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
	encoded, _ := json.Marshal(pausedReasons{PausedReasons: append([]string{}, reasons...)})
	return encoded
}
