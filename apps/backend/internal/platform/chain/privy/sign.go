package privy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const authorizationHeader = "privy-authorization-signature"

func (c *Client) SignTransaction(ctx context.Context, walletID string, unsigned []byte) ([]byte, error) {
	const op = "privy.SignTransaction"
	if c.authKey == nil {
		return nil, errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key missing"))
	}
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/rpc"
	body := map[string]any{
		"method": "signTransaction",
		"params": map[string]any{"transaction": base64.StdEncoding.EncodeToString(unsigned), "encoding": "base64"},
	}
	sig := AuthorizationSignature(
		c.authKey,
		http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+path,
		body,
		c.cfg.AppID,
	)
	var w struct {
		Data struct {
			SignedTransaction string `json:"signed_transaction"`
		} `json:"data"`
	}
	in := call{
		op:      op,
		method:  http.MethodPost,
		path:    path,
		body:    body,
		headers: map[string]string{authorizationHeader: sig},
	}
	if err := c.do(ctx, in, &w); err != nil {
		return nil, err
	}
	signed, err := base64.StdEncoding.DecodeString(w.Data.SignedTransaction)
	if err != nil || len(signed) == 0 {
		return nil, errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	}
	return signed, nil
}

func AuthorizationSignature(key *ecdsa.PrivateKey, method, fullURL string, body any, appID string) string {
	digest := sha256.Sum256(Canonical(map[string]any{
		"version": 1, "method": method, "url": fullURL, "body": body,
		"headers": map[string]string{"privy-app-id": appID},
	}))
	sig, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
	return base64.StdEncoding.EncodeToString(sig)
}

func Canonical(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
