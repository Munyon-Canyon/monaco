package fakes

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type rpcCall struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func routeOf(upstream string, r *http.Request) (string, []string) {
	if route, keys, ok := foldedRoute(upstream, r); ok {
		return route, keys
	}
	if route, keys, ok := foldedAblyRoute(upstream, r); ok {
		return route, keys
	}
	route := "/" + upstream + r.URL.Path
	if upstream != "rpc" || r.Method != http.MethodPost || r.URL.Path != "/" {
		return route, []string{route}
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var c rpcCall
	if json.Unmarshal(body, &c) != nil || c.Method == "" {
		return route, []string{route}
	}
	route = "/rpc/" + c.Method
	var first string
	if len(c.Params) > 0 && json.Unmarshal(c.Params[0], &first) == nil && first != "" {
		return route, []string{route + "/" + first, route}
	}
	return route, []string{route}
}

func sendTransaction(w http.ResponseWriter, r *http.Request) {
	var c rpcCall
	var encoded string
	err := json.NewDecoder(r.Body).Decode(&c)
	if err == nil && len(c.Params) > 0 {
		err = json.Unmarshal(c.Params[0], &encoded)
	}
	tx, _ := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(tx) < 1+ed25519.SignatureSize || tx[0] == 0 {
		rpcReply(w, c.ID, nil, &rpcFault{Code: -32602, Message: "invalid transaction"})
		return
	}
	rpcReply(w, c.ID, chain.SignatureOf(tx[1:1+ed25519.SignatureSize]), nil)
}

type rpcFault struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcReply(w http.ResponseWriter, id json.RawMessage, result any, fault *rpcFault) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result,omitempty"`
		Error   *rpcFault       `json:"error,omitempty"`
	}{"2.0", id, result, fault})
}

func (s *Server) answerRPC(
	w http.ResponseWriter, r *http.Request, route string, keys []string, scriptedFixture bool,
) bool {
	switch route {
	case "/rpc/getMultipleAccounts":
		return s.multipleAccounts(w, r)
	case "/rpc/getTokenAccountsByOwner":
		return !scriptedFixture && s.tokenAccounts(w, r, keys)
	}
	return false
}

type tokenBalance struct {
	amount   uint64
	decimals uint8
}

type SetBalance struct {
	Owner    string `json:"owner"`
	Mint     string `json:"mint"`
	Amount   uint64 `json:"amount,string"`
	Decimals uint8  `json:"decimals"`
}

func (s *Server) setBalance(w http.ResponseWriter, r *http.Request) {
	var b SetBalance
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := chain.ParseAddress(b.Owner); err != nil {
		http.Error(w, fieldError("owner").Error(), http.StatusBadRequest)
		return
	}
	if _, err := chain.ParseAddress(b.Mint); err != nil {
		http.Error(w, fieldError("mint").Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.balances[b.Owner] == nil {
		s.balances[b.Owner] = map[string]tokenBalance{}
	}
	s.balances[b.Owner][b.Mint] = tokenBalance{amount: b.Amount, decimals: b.Decimals}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tokenAccounts(w http.ResponseWriter, r *http.Request, keys []string) bool {
	var call rpcCall
	var owner string
	var filter struct {
		Mint string `json:"mint"`
	}
	if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) < 2 ||
		json.Unmarshal(call.Params[0], &owner) != nil || json.Unmarshal(call.Params[1], &filter) != nil {
		return false
	}
	s.mu.Lock()
	held, set := s.balances[owner]
	balance, ok := held[filter.Mint]
	s.mu.Unlock()
	if !set {
		return s.fixtureTokenAccounts(w, call.ID, keys, filter.Mint)
	}
	accounts := []any{}
	if ok {
		ata, err := chain.AssociatedTokenAccount(
			chain.SolanaAddress(owner), chain.SolanaAddress(filter.Mint), chain.SPLProgram,
		)
		if err != nil {
			rpcReply(w, call.ID, nil, &rpcFault{Code: -32602, Message: "invalid owner or mint"})
			return true
		}
		accounts = append(accounts, map[string]any{"pubkey": ata, "account": map[string]any{
			"lamports": 2039280, "owner": chain.SPLProgram, "data": map[string]any{
				"program": "spl-token", "parsed": map[string]any{"type": "account", "info": map[string]any{
					"mint": filter.Mint, "owner": owner, "state": "initialized",
					"tokenAmount": map[string]any{
						"amount": strconv.FormatUint(balance.amount, 10), "decimals": balance.decimals,
					},
				}},
			},
		}})
	}
	rpcReply(w, call.ID, map[string]any{"context": map[string]any{"slot": 451000000}, "value": accounts}, nil)
	return true
}

func (s *Server) fixtureTokenAccounts(w http.ResponseWriter, id json.RawMessage, keys []string, mint string) bool {
	for _, key := range keys {
		f, ok := s.fixtures[key]
		if !ok {
			continue
		}
		var reply struct {
			Result struct {
				Context json.RawMessage   `json:"context"`
				Value   []json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if f.Status != http.StatusOK || json.Unmarshal(f.Body, &reply) != nil {
			return false
		}
		accounts := []json.RawMessage{}
		for _, raw := range reply.Result.Value {
			var acct struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								Mint string `json:"mint"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			}
			if json.Unmarshal(raw, &acct) == nil && acct.Account.Data.Parsed.Info.Mint == mint {
				accounts = append(accounts, raw)
			}
		}
		rpcReply(w, id, map[string]any{"context": reply.Result.Context, "value": accounts}, nil)
		return true
	}
	return false
}
