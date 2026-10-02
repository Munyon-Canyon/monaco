package fakes

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

func PrivyTokenKey() *ecdsa.PrivateKey { return fixtureP256("monaco-fakes/privy-es256") }

func PrivyVerificationKey() string { return privy.FixtureVerificationKey }

func OtherPrivyVerificationKey() string {
	der, _ := x509.MarshalPKIXPublicKey(&fixtureP256("monaco-fakes/privy-other").PublicKey)
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

type privyCreatedUser struct {
	ID    string
	Email string
}

func (s *Server) privyUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	created, ok := s.createdUsers[id]
	s.mu.Unlock()
	if ok {
		writeJSON(w, map[string]any{
			"id": created.ID, "linked_accounts": []map[string]string{{"type": "email", "address": created.Email}},
		})
		return
	}
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

func (s *Server) privyCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LinkedAccounts []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"linked_accounts"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || len(req.LinkedAccounts) != 1 ||
		req.LinkedAccounts[0].Type != "email" || req.LinkedAccounts[0].Address == "" {
		privyError(w, http.StatusBadRequest, "invalid user request")
		return
	}
	s.mu.Lock()
	s.nextUser++
	id := "did:privy:fake-" + strconv.Itoa(s.nextUser)
	s.createdUsers[id] = privyCreatedUser{ID: id, Email: req.LinkedAccounts[0].Address}
	s.mu.Unlock()
	writeJSON(w, map[string]string{"id": id})
}

func privyError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func FixtureKey(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("monaco-fixture/" + label))
	return ed25519.NewKeyFromSeed(seed[:])
}
