package fakes

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/google/uuid"
)

type privyAutomation struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	OwnerID string `json:"owner_id"`
	key     string
}

type privyAttachment struct {
	ID           string            `json:"id"`
	AutomationID string            `json:"automation_id"`
	WalletID     string            `json:"wallet_id"`
	Params       map[string]string `json:"params"`
	Status       string            `json:"status"`
}

func (s *Server) privyListAutomations(w http.ResponseWriter, _ *http.Request) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	writeJSON(w, map[string]any{"data": append([]privyAutomation{}, p.automations...), "next_cursor": nil})
}

func (s *Server) privyCreateAutomation(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("privy-idempotency-key")
	var req struct {
		Name    string          `json:"name"`
		OwnerID string          `json:"owner_id"`
		Config  json.RawMessage `json:"config"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.OwnerID == "" || len(req.Config) == 0 || key == "" {
		privyError(w, http.StatusBadRequest, "invalid automation request")
		return
	}
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range p.automations {
		if a.key == key {
			writeJSON(w, a)
			return
		}
	}
	a := privyAutomation{ID: uuid.NewString(), Name: req.Name, Status: "enabled", OwnerID: req.OwnerID, key: key}
	p.automations = append(p.automations, a)
	writeJSON(w, a)
}

func (s *Server) privyAttachAutomation(w http.ResponseWriter, r *http.Request) {
	body, ok := signedBody(r, true)
	if !ok {
		privyError(w, http.StatusUnauthorized, "invalid authorization signature")
		return
	}
	var req struct {
		AutomationIDs []string          `json:"automation_ids"`
		Params        map[string]string `json:"params"`
	}
	raw, _ := json.Marshal(body)
	if json.Unmarshal(raw, &req) != nil || len(req.AutomationIDs) == 0 || req.Params["destination_address"] == "" {
		privyError(w, http.StatusBadRequest, "invalid attach request")
		return
	}
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	if p.conflicts(r.PathValue("id")+"/"+r.Header.Get("privy-idempotency-key"), body) {
		privyError(w, http.StatusBadRequest, "idempotency key reused with a different request")
		return
	}
	data := []privyAttachment{}
	for _, id := range req.AutomationIDs {
		if i := slices.IndexFunc(p.attachments, func(at privyAttachment) bool {
			return at.WalletID == r.PathValue("id") && at.AutomationID == id
		}); i >= 0 {
			data = append(data, p.attachments[i])
			continue
		}
		at := privyAttachment{
			ID: uuid.NewString(), AutomationID: id, WalletID: r.PathValue("id"), Params: req.Params, Status: "enabled",
		}
		p.attachments = append(p.attachments, at)
		data = append(data, at)
	}
	writeJSON(w, map[string]any{"data": data})
}

func (s *Server) SeedPrivyExecution(execution map[string]any) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	p.executions = append(p.executions, execution)
}

func (s *Server) privyExecutions(w http.ResponseWriter, r *http.Request) {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	wallet := r.URL.Query().Get("wallet_id")
	var matched []map[string]any
	for _, e := range p.executions {
		if wallet == "" || e["wallet_id"] == wallet {
			matched = append(matched, e)
		}
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 25
	}
	from, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	from = min(max(from, 0), len(matched))
	to := min(from+limit, len(matched))
	var next any
	if to < len(matched) {
		next = strconv.Itoa(to)
	}
	page := append([]map[string]any{}, matched[from:to]...)
	writeJSON(w, map[string]any{"data": page, "next_cursor": next})
}

func (s *Server) privyReindex(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WalletID     string `json:"wallet_id"`
		CAIP2        string `json:"caip2"`
		AssetAddress string `json:"asset_address"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.WalletID == "" || req.AssetAddress == "" {
		privyError(w, http.StatusBadRequest, "invalid reindex request")
		return
	}
	writeJSON(w, map[string]any{"wallet_id": req.WalletID, "results": []map[string]any{{
		"caip2": req.CAIP2, "asset_address": req.AssetAddress, "status": "skipped_zero_balance", "raw_balance": "0",
		"existing_execution_id": nil,
	}}})
}

func (s *Server) PrivyAttachments(walletID string) []map[string]string {
	p := &s.multichain
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]string
	for _, at := range p.attachments {
		if at.WalletID == walletID {
			out = append(out, at.Params)
		}
	}
	return out
}
