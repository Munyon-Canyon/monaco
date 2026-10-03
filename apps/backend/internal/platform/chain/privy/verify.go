package privy

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const issuer = "privy.io"

type claims struct {
	Iss string   `json:"iss"`
	Sub string   `json:"sub"`
	Aud audience `json:"aud"`
	Exp int64    `json:"exp"`
	Nbf int64    `json:"nbf"`
}

type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errs.Wrap(err, errs.CodeUnauthorized, "privy.audience")
	}
	*a = many
	return nil
}

func (c *Client) VerifyAccessToken(_ context.Context, raw string) (UserID, error) {
	const op = "privy.VerifyAccessToken"
	payload, reason := c.verified(raw)
	if reason != "" {
		return "", errs.New(errs.CodeUnauthorized, op, slog.String("reason", reason))
	}
	var cl claims
	if err := json.Unmarshal(payload, &cl); err != nil {
		return "", errs.New(errs.CodeUnauthorized, op, slog.String("reason", "claims"))
	}
	now := c.clock.Now().Unix()
	switch {
	case cl.Iss != issuer:
		reason = "issuer"
	case !slices.Contains(cl.Aud, c.cfg.AppID):
		reason = "audience"
	case now >= cl.Exp:
		reason = "expired"
	case now < cl.Nbf:
		reason = "not yet valid"
	case cl.Sub == "":
		reason = "subject"
	default:
		return UserID(cl.Sub), nil
	}
	return "", errs.New(errs.CodeUnauthorized, op, slog.String("reason", reason))
}

func (c *Client) verified(raw string) ([]byte, string) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, "malformed"
	}
	enc := base64.RawURLEncoding
	header, errH := enc.DecodeString(parts[0])
	payload, errP := enc.DecodeString(parts[1])
	sig, errS := enc.DecodeString(parts[2])
	var h struct {
		Alg string `json:"alg"`
	}
	if errH != nil || errP != nil || errS != nil || json.Unmarshal(header, &h) != nil {
		return nil, "malformed"
	}
	if h.Alg != "ES256" || len(sig) != 64 {
		return nil, "algorithm"
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(c.verifyKey, digest[:], r, s) {
		return nil, "signature"
	}
	return payload, ""
}
