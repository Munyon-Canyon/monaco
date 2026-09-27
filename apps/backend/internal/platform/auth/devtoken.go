package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const hs256Header = `{"alg":"HS256","typ":"JWT"}`

type TokenVerifier interface {
	Verify(ctx context.Context, raw string) (Actor, error)
}

type DevVerifier struct {
	key   []byte
	clock clock.Clock
}

type devClaims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

func NewDevVerifier(cfg config.Config, c clock.Clock) (*DevVerifier, error) {
	const op = "auth.NewDevVerifier"
	if cfg.Env == config.EnvProduction {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("env", string(cfg.Env)))
	}
	if cfg.Auth.DevTokenKey == "" {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("missing", "MONACO_DEV_TOKEN_KEY"))
	}
	return &DevVerifier{key: []byte(cfg.Auth.DevTokenKey), clock: c}, nil
}

func (v *DevVerifier) Mint(userID string, exp time.Time) string {
	claims, _ := json.Marshal(devClaims{Sub: userID, Exp: exp.Unix()})
	signing := encodeSegment([]byte(hs256Header)) + "." + encodeSegment(claims)
	return signing + "." + encodeSegment(v.sign(signing))
}

func (v *DevVerifier) Verify(_ context.Context, raw string) (Actor, error) {
	const op = "auth.DevVerifier.Verify"
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "malformed"))
	}
	header, err := decodeSegment(parts[0])
	if err != nil || string(header) != hs256Header {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "unsupported_header"))
	}
	sig, err := decodeSegment(parts[2])
	if err != nil || !hmac.Equal(sig, v.sign(parts[0]+"."+parts[1])) {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "bad_signature"))
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "malformed"))
	}
	var claims devClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Sub == "" {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "no_subject"))
	}
	if !v.clock.Now().Before(time.Unix(claims.Exp, 0)) {
		return Actor{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "expired"))
	}
	return Actor{Kind: ActorUser, ID: claims.Sub}, nil
}

func (v *DevVerifier) sign(signing string) []byte {
	mac := hmac.New(sha256.New, v.key)
	_, _ = mac.Write([]byte(signing))
	return mac.Sum(nil)
}

func encodeSegment(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func decodeSegment(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUnauthorized, "auth.decodeSegment")
	}
	return b, nil
}
