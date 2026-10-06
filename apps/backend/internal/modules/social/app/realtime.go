package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Realtime interface {
	Publish(ctx context.Context, channel string, name string, data any) error
	TokenRequest(ctx context.Context, clientID ids.UserID, channels []string, ttl time.Duration) (TokenRequest, error)
}

type TokenRequest struct {
	KeyName    string
	ClientID   string
	Capability string
	Timestamp  int64
	TTL        int64
	Nonce      string
	MAC        string
}
