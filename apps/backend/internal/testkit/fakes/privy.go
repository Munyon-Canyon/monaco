package fakes

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strings"
	"time"
)

func PrivyTokenKey() *ecdsa.PrivateKey { return fixtureP256("monaco-fakes/privy-es256") }

func PrivyVerificationKey() string {
	der, _ := x509.MarshalPKIXPublicKey(&PrivyTokenKey().PublicKey)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func PrivyAccessToken(appID, sub string, issued time.Time, ttl time.Duration) string {
	claims, _ := json.Marshal(map[string]any{
		"iss": "privy.io", "aud": appID, "sub": sub, "sid": "session-" + sub,
		"iat": issued.Unix(), "exp": issued.Add(ttl).Unix(),
	})
	return SignES256(PrivyTokenKey(), `{"alg":"ES256","typ":"JWT"}`, string(claims))
}

func SignES256(key *ecdsa.PrivateKey, header, claims string) string {
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(claims))
	digest := sha256.Sum256([]byte(signing))
	r, s, _ := ecdsa.Sign(rand.Reader, key, digest[:])
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig)
}

func fixtureP256(label string) *ecdsa.PrivateKey {
	d := sha256.Sum256([]byte(label))
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), d[:])
	if err != nil {
		panic("fakes: fixture P-256 scalar out of range: " + label)
	}
	return key
}

func (s *Server) privyUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for key, f := range s.fixtures {
		var body struct {
			ID string `json:"id"`
		}
		if strings.HasPrefix(key, "/privy/v1/users/") && json.Unmarshal(f.Body, &body) == nil && body.ID == id {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(f.Body)
			return
		}
	}
	privyError(w, http.StatusNotFound, "User not found")
}

func privyError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
