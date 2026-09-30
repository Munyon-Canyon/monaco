package privy

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const FixtureVerificationKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEX8TGyTbg95Nndd3sWnscZOdcozx4
PbZr5dHJcQABemIlgpLsqqdXw+/GD22Prw6I8KsBYFpSnJHaFWpMfsAXGw==
-----END PUBLIC KEY-----
`

func CheckVerificationKey(cfg config.Config) error {
	key := publicKey(cfg.Privy.VerificationKey)
	if cfg.Env.Deployed() && key != nil && key.Equal(publicKey(FixtureVerificationKey)) {
		return errs.New(errs.CodeInvalidConfig, "privy.CheckVerificationKey",
			slog.String("reason", "PRIVY_VERIFICATION_KEY is the fixture key, which is for tests only"))
	}
	return nil
}
