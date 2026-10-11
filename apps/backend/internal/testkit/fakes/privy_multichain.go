package fakes

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

type privyAction struct {
	ID          string           `json:"id"`
	Status      string           `json:"status"`
	WalletID    string           `json:"wallet_id"`
	Type        string           `json:"type"`
	SourceChain string           `json:"source_chain"`
	Destination string           `json:"destination_address"`
	DestChain   string           `json:"destination_chain"`
	Amount      string           `json:"source_amount"`
	Steps       []map[string]any `json:"steps,omitempty"`
}

type privyMultichain struct {
	mu          sync.Mutex
	actions     map[string]privyAction
	byKey       map[string]string
	automations []privyAutomation
	attachments []privyAttachment
	executions  []map[string]any
	bodies      map[string]string
}

func (p *privyMultichain) init() {
	if p.actions == nil {
		p.actions, p.byKey, p.bodies = map[string]privyAction{}, map[string]string{}, map[string]string{}
	}
}

func (p *privyMultichain) conflicts(key string, body map[string]any) bool {
	raw, _ := json.Marshal(body)
	prior, ok := p.bodies[key]
	if !ok {
		p.bodies[key] = string(raw)
	}
	return ok && prior != string(raw)
}

func (s *Server) privyCreateAnyWallet(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req struct {
		ChainType string `json:"chain_type"`
		OwnerID   string `json:"owner_id"`
	}
	_ = json.Unmarshal(raw, &req)
	if req.ChainType != "ethereum" {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		s.privyCreateWallet(w, r)
		return
	}
	key := r.Header.Get("privy-idempotency-key")
	if key == "" || req.OwnerID == "" {
		privyError(w, http.StatusBadRequest, "invalid wallet request")
		return
	}
	sum := sha256.Sum256([]byte(key))
	id := "wallet-evm-" + hex.EncodeToString(sum[:6])
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.wallets[id]; ok {
		writeJSON(w, existing)
		return
	}
	addr := sha256.Sum256([]byte("monaco-fakes/privy-evm-wallet/" + id))
	wallet := privyWallet{
		ID:        id,
		ChainType: "ethereum",
		OwnerID:   req.OwnerID,
		Address:   "0x" + hex.EncodeToString(addr[:20]),
	}
	s.wallets[id] = wallet
	writeJSON(w, wallet)
}

func signedBody(r *http.Request, needKey bool) (map[string]any, bool) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&body) != nil {
		return nil, false
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	headers := map[string]string{"privy-app-id": r.Header.Get("privy-app-id")}
	key := r.Header.Get("privy-idempotency-key")
	if key == "" && needKey {
		return nil, false
	}
	if key != "" {
		headers["privy-idempotency-key"] = key
	}
	payload := privy.Canonical(map[string]any{
		"version": 1, "method": r.Method, "url": "http://" + host + r.URL.Path, "body": body, "headers": headers,
	})
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get("privy-authorization-signature"))
	digest := sha256.Sum256(payload)
	return body, err == nil && ecdsa.VerifyASN1(&PrivyAuthorizationKey().PublicKey, digest[:], sig)
}

func (s *Server) privyTransfer(w http.ResponseWriter, r *http.Request) {
	body, ok := signedBody(r, true)
	if !ok {
		privyError(w, http.StatusUnauthorized, "invalid authorization signature")
		return
	}
	var req struct {
		Amount string `json:"amount"`
		Source struct {
			Asset string `json:"asset"`
			Chain string `json:"chain"`
		} `json:"source"`
		Destination struct {
			Address string `json:"address"`
			Chain   string `json:"chain"`
		} `json:"destination"`
	}
	raw, _ := json.Marshal(body)
	if json.Unmarshal(raw, &req) != nil || req.Amount == "" || req.Source.Asset != "usdc" ||
		req.Destination.Address == "" {
		privyError(w, http.StatusBadRequest, "invalid transfer request")
		return
	}
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	key := r.PathValue("id") + "/" + r.Header.Get("privy-idempotency-key")
	if id, ok := p.byKey[key]; ok {
		writeJSON(w, p.actions[id])
		return
	}
	action := privyAction{
		ID: uuid.NewString(), Status: "pending", WalletID: r.PathValue("id"), Type: "transfer",
		SourceChain: req.Source.Chain, Destination: req.Destination.Address, DestChain: req.Destination.Chain,
		Amount: req.Amount,
	}
	p.actions[action.ID], p.byKey[key] = action, action.ID
	writeJSON(w, action)
}

func (s *Server) privyAction(w http.ResponseWriter, r *http.Request) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	action, ok := p.actions[r.PathValue("action")]
	if !ok || action.WalletID != r.PathValue("id") {
		privyError(w, http.StatusNotFound, "wallet action not found")
		return
	}
	action.Status = "succeeded"
	sum := sha256.Sum256([]byte(action.ID))
	source := chain.Network(action.SourceChain)
	first := map[string]any{
		"type": "svm_transaction", "status": "confirmed", "caip2": source.CAIP2(),
		"transaction_signature": string(chain.AddressOf(sum[:])),
	}
	if source.IsEVM() {
		first = map[string]any{
			"type": "evm_transaction", "status": "confirmed", "caip2": source.CAIP2(),
			"transaction_hash": "0x" + hex.EncodeToString(sum[:]),
		}
	}
	action.Steps = []map[string]any{first}
	if action.DestChain != "" && action.DestChain != action.SourceChain {
		action.Steps = append(action.Steps, map[string]any{
			"type": "external_transaction", "status": "confirmed",
		})
	}
	p.actions[action.ID] = action
	writeJSON(w, action)
}
