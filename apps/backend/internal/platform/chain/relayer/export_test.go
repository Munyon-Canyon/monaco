package relayer

import (
	"crypto/ed25519"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func WithKey(r *Relayer, key ed25519.PrivateKey) *Relayer {
	c := *r
	c.key = key
	return &c
}

func InspectMessage(msg []byte) error { return inspectSwap(chain.Transaction{Message: msg}) }
