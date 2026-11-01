package relayer

import "crypto/ed25519"

func WithKey(r *Relayer, key ed25519.PrivateKey) *Relayer {
	c := *r
	c.key = key
	return &c
}

func Inspect(l Limits, msg []byte) (Spend, error) { return l.inspect(msg) }
