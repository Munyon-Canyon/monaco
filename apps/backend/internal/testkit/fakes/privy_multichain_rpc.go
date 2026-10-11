package fakes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) SetPrivyActionOutcome(status string) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outcome = status
}

func (s *Server) setPrivyActionOutcome(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil ||
		(req.Status != "" && req.Status != "succeeded" && req.Status != "rejected" && req.Status != "failed") {
		http.Error(w, "status is succeeded, rejected or failed", http.StatusBadRequest)
		return
	}
	s.SetPrivyActionOutcome(req.Status)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) PrivyEVMSends() int {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sends)
}

func (s *Server) privyRPC(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var head struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(raw, &head)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if !strings.HasPrefix(head.Method, "eth_") {
		s.privySign(w, r)
		return
	}
	body, ok := signedBody(r, head.Method == "eth_sendTransaction")
	if !ok {
		privyError(w, http.StatusUnauthorized, "invalid authorization signature")
		return
	}
	var req struct {
		CAIP2   string `json:"caip2"`
		Sponsor bool   `json:"sponsor"`
		Params  struct {
			Transaction struct {
				To string `json:"to"`
			} `json:"transaction"`
		} `json:"params"`
	}
	_ = json.Unmarshal(raw, &req)
	if !strings.HasPrefix(req.CAIP2, "eip155:") || req.Params.Transaction.To == "" {
		privyError(w, http.StatusBadRequest, "invalid ethereum request")
		return
	}
	sum := sha256.Sum256(raw)
	if head.Method == "eth_signTransaction" {
		writeJSON(w, map[string]any{"method": head.Method, "data": map[string]string{
			"signed_transaction": "0x02" + hex.EncodeToString(sum[:]), "encoding": "rlp",
		}})
		return
	}
	s.privySend(w, r, body, req.Sponsor)
}

func (s *Server) privySend(w http.ResponseWriter, r *http.Request, body map[string]any, sponsor bool) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	key := r.PathValue("id") + "/" + r.Header.Get("privy-idempotency-key")
	if p.conflicts(key, body) {
		privyError(w, http.StatusBadRequest, "idempotency key reused with a different request")
		return
	}
	if _, ok := p.sends[key]; !ok {
		sum := sha256.Sum256([]byte(key))
		data := map[string]any{"hash": "0x" + hex.EncodeToString(sum[:]), "caip2": body["caip2"]}
		if sponsor {
			data["hash"], data["user_operation_hash"] = "", "0x"+hex.EncodeToString(sum[:])
		}
		p.sends[key] = data
	}
	writeJSON(w, map[string]any{"method": "eth_sendTransaction", "data": p.sends[key]})
}
