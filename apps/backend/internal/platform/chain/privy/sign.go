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
	var w struct {
		Data struct {
			SignedTransaction string `json:"signed_transaction"`
		} `json:"data"`
	}
	if err := c.do(ctx, c.signed(op, http.MethodPost, path, body, ""), &w); err != nil {
		return nil, err
	}
	signed, err := base64.StdEncoding.DecodeString(w.Data.SignedTransaction)
	if err != nil || len(signed) == 0 {
		return nil, errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	}
	return signed, nil
}

func (c *Client) signed(op, method, path string, body any, idemKey string) call {
	headers := map[string]string{"privy-app-id": c.cfg.AppID}
	if idemKey != "" {
		headers[idempotencyHeader] = idemKey
	}
	sig := AuthorizationSignature(c.authKey, method, strings.TrimRight(c.cfg.BaseURL, "/")+path, generic(body), headers)
	out := call{op: op, method: method, path: path, body: body, headers: map[string]string{authorizationHeader: sig}}
	if idemKey != "" {
		out.headers[idempotencyHeader] = idemKey
	}
	return out
}

func AuthorizationSignature(key *ecdsa.PrivateKey, method, fullURL string, body any, headers map[string]string) string {
	digest := sha256.Sum256(Canonical(map[string]any{
		"version": 1, "method": method, "url": fullURL, "body": body, "headers": headers,
	}))
	sig, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
	return base64.StdEncoding.EncodeToString(sig)
}

func generic(body any) any {
	raw, _ := json.Marshal(body)
	var out any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return out
}

func Canonical(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
