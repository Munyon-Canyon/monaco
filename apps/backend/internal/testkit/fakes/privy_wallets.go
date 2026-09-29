package fakes

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

const PrivyAuthorizationKeyID = "fixture-key-quorum"

type privySigner struct {
	SignerID string `json:"signer_id"`
}

type privyWallet struct {
	ID                string        `json:"id"`
	Address           string        `json:"address"`
	ChainType         string        `json:"chain_type"`
	OwnerID           string        `json:"owner_id"`
	AdditionalSigners []privySigner `json:"additional_signers"`
}

func PrivyAuthorizationKey() *ecdsa.PrivateKey {
	return fixtureP256("monaco-fakes/privy-authorization")
}

func PrivyAuthorizationKeyConfig() string {
	der, _ := x509.MarshalPKCS8PrivateKey(PrivyAuthorizationKey())
	return "wallet-auth:" + base64.StdEncoding.EncodeToString(der)
}

func PrivyWalletKey(walletID string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("monaco-fakes/privy-wallet/" + walletID))
	return ed25519.NewKeyFromSeed(seed[:])
}

func (s *Server) privyWallet(id string) (privyWallet, bool) {
	s.mu.Lock()
	w, ok := s.wallets[id]
	s.mu.Unlock()
	if ok {
		return w, true
	}
	f, ok := s.fixtures["/privy/v1/wallets/"+id]
	return w, ok && json.Unmarshal(f.Body, &w) == nil
}

func (s *Server) privyWallets(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user_id")
	data := []privyWallet{}
	for key := range s.fixtures {
		if id, ok := strings.CutPrefix(key, "/privy/v1/wallets/"); ok {
			if wallet, _ := s.privyWallet(id); wallet.OwnerID == user {
				data = append(data, wallet)
			}
		}
	}
	s.mu.Lock()
	for _, wallet := range s.wallets {
		if wallet.OwnerID == user {
			data = append(data, wallet)
		}
	}
	s.mu.Unlock()
	slices.SortFunc(data, func(a, b privyWallet) int { return strings.Compare(a.ID, b.ID) })
	writeJSON(w, map[string]any{"data": data, "next_cursor": nil})
}

func (s *Server) privyCreateWallet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChainType         string            `json:"chain_type"`
		Owner             map[string]string `json:"owner"`
		OwnerID           string            `json:"owner_id"`
		AdditionalSigners []privySigner     `json:"additional_signers"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.ChainType != "solana" {
		privyError(w, http.StatusBadRequest, "invalid wallet request")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "wallet-" + strconv.Itoa(len(s.wallets)+1)
	if key := r.Header.Get("privy-idempotency-key"); key != "" {
		sum := sha256.Sum256([]byte(key))
		id = "wallet-" + hex.EncodeToString(sum[:6])
	}
	if existing, ok := s.wallets[id]; ok {
		writeJSON(w, existing)
		return
	}
	owner := req.OwnerID
	if owner == "" {
		owner = req.Owner["user_id"]
	}
	wallet := privyWallet{
		ID: id, ChainType: "solana", OwnerID: owner, AdditionalSigners: req.AdditionalSigners,
		Address: string(chain.AddressOf(PrivyWalletKey(id).Public().(ed25519.PublicKey))),
	}
	s.wallets[id] = wallet
	writeJSON(w, wallet)
}

func (s *Server) privySign(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&body) != nil || !authorized(r, body) {
		privyError(w, http.StatusUnauthorized, "invalid authorization signature")
		return
	}
	id := r.PathValue("id")
	if _, ok := s.privyWallet(id); !ok {
		privyError(w, http.StatusNotFound, "wallet not found")
		return
	}
	var req struct {
		Method string `json:"method"`
		Params struct {
			Transaction string `json:"transaction"`
		} `json:"params"`
	}
	_ = json.Unmarshal(raw, &req)
	unsigned, _ := base64.StdEncoding.DecodeString(req.Params.Transaction)
	tx, err := chain.DecodeTransaction(unsigned)
	if err == nil {
		err = tx.Sign(PrivyWalletKey(id))
	}
	if req.Method != "signTransaction" || err != nil {
		privyError(w, http.StatusBadRequest, "wallet cannot sign this transaction")
		return
	}
	writeJSON(w, map[string]any{
		"method": "signTransaction",
		"data": map[string]string{
			"signed_transaction": base64.StdEncoding.EncodeToString(tx.Encode()),
			"encoding":           "base64",
		},
	})
}

func authorized(r *http.Request, body map[string]any) bool {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	payload := privy.Canonical(map[string]any{
		"version": 1, "method": r.Method, "url": "http://" + host + r.URL.Path, "body": body,
		"headers": map[string]string{"privy-app-id": r.Header.Get("privy-app-id")},
	})
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get("privy-authorization-signature"))
	digest := sha256.Sum256(payload)
	return err == nil && ecdsa.VerifyASN1(&PrivyAuthorizationKey().PublicKey, digest[:], sig)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
