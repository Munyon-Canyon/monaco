package agents

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func (env *Env) verifierKey(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env.Home == "" {
		return "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.verifierKey", "HOME is unset")
	}
	return filepath.Join(env.Home, ".config", "monaco", "verifier.pem"), nil
}

func (env *Env) statusAuth(ctx context.Context, keyFlag string) (string, error) {
	path, err := env.verifierKey(keyFlag)
	if err != nil {
		return "", err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		t, err := env.GitHub.Token(ctx)
		if err != nil {
			return "", err
		}
		return "token " + t, nil
	}
	if err != nil {
		return "", fmt.Errorf("verifier key: %w", err)
	}
	key, err := loadKey(path)
	if err != nil {
		return "", err
	}
	signed, err := signJWT(key, env.Config.VerifierApp, env.Now())
	if err != nil {
		return "", err
	}
	var resp struct {
		Token string `json:"token"`
	}
	path = fmt.Sprintf("/app/installations/%d/access_tokens", env.Config.VerifierInstallation)
	if err := env.GitHub.call(ctx, "POST", path, "Bearer "+signed, map[string]any{}, &resp); err != nil {
		return "", err
	}
	if resp.Token == "" {
		return "", detailErr(errs.CodeUnauthorized, "monacoctl.agents.statusAuth", "installation token was empty")
	}
	return "token " + resp.Token, nil
}

func loadKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verifier key: %w", err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, detailErr(errs.CodeDecodeFailed, "monacoctl.agents.loadKey", "verifier key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, detailErr(errs.CodeDecodeFailed, "monacoctl.agents.loadKey", "verifier key is not PKCS1 or PKCS8")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, detailErr(errs.CodeDecodeFailed, "monacoctl.agents.loadKey", "verifier key is not RSA")
	}
	return key, nil
}

func signJWT(key *rsa.PrivateKey, iss string, now time.Time) (string, error) {
	payload := fmt.Sprintf(`{"iat":%d,"exp":%d,"iss":"%s"}`, now.Unix()-60, now.Unix()+540, iss)
	unsigned := b64(`{"alg":"RS256","typ":"JWT"}`) + "." + b64(payload)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign verifier jwt: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func pemBlock(typ string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}
