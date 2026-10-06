package exports

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

const authStateCreated = "CREATED"

func UserSignedUp(_ context.Context, e events.UserCreated) (app.Capture, bool, error) {
	return app.Capture{
		Event: "user_signed_up", DistinctID: e.UserID.String(),
		Properties: map[string]any{"login_provider": e.LoginProvider},
		Set: map[string]any{
			"login_provider": e.LoginProvider, "created_at": e.CreatedAt.UTC(), "auth_state": authStateCreated,
		},
	}, true, nil
}

func AuthStateChanged(_ context.Context, e events.UserAuthStateChanged) (app.Capture, bool, error) {
	return app.Capture{
		Event: "auth_state_changed", DistinctID: e.UserID.String(),
		Properties: map[string]any{"from": e.From, "to": e.To, "cause": e.Cause},
		Set:        map[string]any{"auth_state": e.To},
	}, true, nil
}
